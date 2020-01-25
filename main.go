package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"log"
	"os"
	"path"

	"github.com/google/uuid"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/partition/mbr"
)

type middleFileReader struct {
	os.File
	Start uint32
	Size  uint32
	total uint64
}

func (m middleFileReader) Read(p []byte) (int, error) {
	n, err := m.File.ReadAt(p, int64(m.Start+uint32(m.total)))
	if err != nil {
		return n, err
	}

	tmpTotal := uint64(n) + m.total

	if uint32(tmpTotal) >= m.Size {
		return int((m.Start + m.Size) - (m.Start + uint32(m.total))), io.EOF
	}
	m.total = m.total + uint64(n)

	return n, nil
}

func main() {

	var imagePath string
	var diskName string

	flag.StringVar(&imagePath, "image", "", "The path to the image")
	flag.StringVar(&diskName, "disk", "", "The path to the disk to write the image to")

	flag.Parse()

	log.Printf("Reading image %s", imagePath)
	imageDisk, err := diskfs.Open(imagePath)
	if err != nil {
		log.Fatalf("Error opening image %s: %v", imagePath, err)
	}
	rawPartitions, err := imageDisk.GetPartitionTable()
	imagePartitionTable := rawPartitions.(*mbr.Table)
	imagePartitions := imagePartitionTable.Partitions

	log.Printf("Reading disk %s", diskName)
	destDisk, err := diskfs.Open(diskName)
	if err != nil {
		log.Fatalf("Error opening disk %s: %v", diskName, err)
	}

	startSector := uint32(2048)
	cloudInitSize := 1 * 1024 * 1024 * 1024 // 1 GB
	cloudInitSectors := uint32(cloudInitSize / int(destDisk.LogicalBlocksize))

	table := &mbr.Table{
		LogicalSectorSize:  int(destDisk.LogicalBlocksize),
		PhysicalSectorSize: int(destDisk.PhysicalBlocksize),
		Partitions: []*mbr.Partition{
			{
				Bootable: false,
				Type:     mbr.Linux,
				Start:    startSector,
				Size:     cloudInitSectors,
			},
		},
	}

	nextSectorStart := startSector + cloudInitSectors

	// copy partition table from image
	log.Print("Copying partition table from image")
	for _, partition := range imagePartitions {
		log.Printf("Copying partition info %v", partition)
		if partition.Type == mbr.Empty {
			log.Printf("Ignoring empty typed partition")
			// ignore partitions that are empty type
			continue
		}
		table.Partitions = append(table.Partitions, &mbr.Partition{
			Bootable: partition.Bootable,
			Type:     partition.Type,
			Start:    nextSectorStart,
			Size:     partition.Size,
		})
		nextSectorStart = nextSectorStart + partition.Size
	}

	// write partition table to disk
	log.Print("Writing partition table to disk")
	err = destDisk.Partition(table)
	if err != nil {
		log.Fatalf("Error writing partition table to disk %s: %v", diskName, err)
	}

	log.Print("Copying partition contents from image")
	for i, partition := range imagePartitions {
		if partition.Type == mbr.Empty {
			continue
		}
		f := imageDisk.File
		_, err = destDisk.WritePartitionContents(i+2, middleFileReader{
			File:  *f,
			Start: uint32(imagePartitionTable.LogicalSectorSize) * partition.Start,
			Size:  uint32(imagePartitionTable.LogicalSectorSize) * partition.Size,
		})
		if err != nil {
			log.Fatalf("Error writing partition content from image %v", err)
		}
	}

	log.Printf("Cleaning cloud init partition")
	b := make([]byte, destDisk.LogicalBlocksize*int64(cloudInitSectors))
	_, err = destDisk.WritePartitionContents(1, bytes.NewReader(b))
	if err != nil {
		log.Fatalf("Error cleaning cloud-init partition: %v", err)
	}

	// create the cloud init filesystem
	log.Print("Creating cloud init filesystem")
	cloudInitFS, err := destDisk.CreateFilesystem(disk.FilesystemSpec{
		Partition:   1,
		FSType:      filesystem.TypeFat32,
		VolumeLabel: "config-2",
	})
	if err != nil {
		log.Fatalf("Error creating cloud-init filesystem on %s: %v", diskName, err)
	}

	cloudInitPrefix := path.Join("/", "openstack", "latest")
	// place down cloud-init info
	log.Print("Creating cloud init directory structure")
	err = cloudInitFS.Mkdir(cloudInitPrefix)
	if err != nil {
		log.Fatalf("Error creating cloud init directory structure: %v", err)
	}

	metadataPath := path.Join(cloudInitPrefix, "meta_data.json")
	log.Printf("Opening %s", metadataPath)
	metadataFile, err := cloudInitFS.OpenFile(metadataPath, os.O_CREATE|os.O_RDWR)
	if err != nil {
		log.Fatalf("Error opening meta data: %v", err)
	}
	uid, err := uuid.NewUUID()
	if err != nil {
		log.Fatalf("Error generating metadata uuid %v", err)
	}
	metadataContents := map[string]interface{}{
		"uuid": uid.String(),
		"public_keys": map[string]string{
			"rmb938": "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDB9FH324syhZ88B3TiMkYIMrI2/yvCF+tiWk+eOQKnmxA4zXSeVot1z52fk6P2xdZU9jzni2Qm5PihVKclzQmvIijpXV7MBXQS2/G100FyfZL76LK/ZLGITE3MU2+iBVH59gq+sJywQXkXYLngZiChVbokFidND39kNuQXQZCb2lnKXwM6KLMn4v9nFBTYQmjImqm+2BMsKgdupaYm+qzr+Gr8lLitb+VKJtsrnRaW0NerTLNr3fXtw0sgeQkcQtqaKOvPRocUoa7qnzI0TP8Mx02klTiWwHvPzc9e0HztXOQwYZB6/dcB9CoglLYnzcTf2cEVGHO9NGb9GLqn3Oph",
		},
		"hostname": "my-hostname",
	}
	data, err := json.MarshalIndent(&metadataContents, "", "\t")
	log.Print("Writing metadata contents")
	_, err = metadataFile.Write(data)
	if err != nil {
		log.Fatalf("Error writting meta data: %v", err)
	}

	networkdataPath := path.Join(cloudInitPrefix, "network_data.json")
	log.Printf("Opening %s", networkdataPath)
	networkdataFile, err := cloudInitFS.OpenFile(networkdataPath, os.O_CREATE|os.O_RDWR)
	if err != nil {
		log.Fatalf("Error opening network data: %v", err)
	}
	networkdataContents := map[string]interface{}{
		"links": []map[string]string{
			{
				"id":                   "eth0",
				"ethernet_mac_address": "d0:50:99:d3:47:d1",
				"type":                 "phy",
			},
		},
		"networks": []map[string]interface{}{
			{
				"link":            "eth0",
				"type":            "ipv4",
				"ip_address":      "192.168.23.160",
				"netmask":         "255.255.255.0",
				"gateway":         "192.168.23.254",
				"dns_nameservers": []string{"192.168.23.254"},
				"dns_search":      []string{"rmb938.me"},
			},
		},
	}
	data, err = json.MarshalIndent(&networkdataContents, "", "\t")
	log.Print("Writing networkdata contents")
	_, err = networkdataFile.Write(data)
	if err != nil {
		log.Fatalf("Error writting network data: %v", err)
	}

	userdataPath := path.Join(cloudInitPrefix, "user_data")
	log.Printf("Opening %s", userdataPath)
	userdataFile, err := cloudInitFS.OpenFile(userdataPath, os.O_CREATE|os.O_RDWR)
	if err != nil {
		log.Fatalf("Error opening user data: %v", err)
	}
	_, err = userdataFile.Write([]byte("#cloud-config\n{}"))
	if err != nil {
		log.Fatalf("Error writting user data: %v", err)
	}
}
