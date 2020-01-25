package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"log"
	"os"
	"path"

	"github.com/diskfs/go-diskfs/partition/mbr"
	"github.com/google/uuid"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
)

type middleFileReader struct {
	os.File
	Start uint32
	Size  uint32
	total uint64
}

func (m *middleFileReader) Read(p []byte) (int, error) {
	if uint32(m.total) >= m.Size {
		log.Print("Someone is trying to read after we are done")
		return 0, io.EOF
	}

	n, err := m.File.ReadAt(p, int64(m.Start+uint32(m.total)))
	if err != nil {
		return n, err
	}

	tmpTotal := m.total + uint64(n)

	log.Printf("Start: %v Size: %v Total: %v Read: %v", m.Start, m.Size, tmpTotal, n)

	if uint32(tmpTotal) > m.Size {
		log.Printf("Read more than size so we are done %v", int(m.Size-uint32(m.total)))
		m.total = uint64(m.Size)
		return int(m.Size - uint32(m.total)), io.EOF
	}
	m.total = m.total + uint64(n)

	log.Printf("Start: %v Size: %v Total: %v Read: %v", m.Start, m.Size, m.total, n)

	return n, nil
}

func main() {

	var imagePath string
	var diskPath string

	flag.StringVar(&imagePath, "image", "", "The path to the image")
	flag.StringVar(&diskPath, "disk", "", "The path to the disk to write the image to")

	flag.Parse()

	log.Printf("Reading image %s", imagePath)
	imageFile, err := os.Open(imagePath)
	if err != nil {
		log.Fatalf("Error opening image %s: %v", imagePath, err)
	}

	diskFile, err := os.Open(diskPath)
	if err != nil {
		log.Fatalf("Error opening disk %s: %v", diskPath, err)
	}

	log.Print("Writing image to disk")
	_, err = io.Copy(diskFile, imageFile)
	if err != nil {
		log.Fatalf("Error writing image to disk %s: %v", diskPath, err)
	}
	err = imageFile.Close()
	if err != nil {
		log.Fatalf("Error closing image %s: %v", imagePath, err)
	}
	err = diskFile.Close()
	if err != nil {
		log.Fatalf("Error closing disk %s: %v", diskPath, err)
	}

	log.Printf("Reading disk partitions %s", diskPath)
	destDisk, err := diskfs.Open(diskPath)
	if err != nil {
		log.Fatalf("Error opening disk %s: %v", diskPath, err)
	}

	rawTable, err := destDisk.GetPartitionTable()
	if err != nil {
		log.Fatalf("Error getting partition table for disk %s: %v", diskPath, err)
	}
	table := rawTable.(*mbr.Table)

	cloudInitSize := 1 * 1024 * 1024 * 1024 // 1 GB
	cloudInitSectors := uint32(cloudInitSize / table.LogicalSectorSize)
	cloudInitStart := uint32(int(destDisk.Size)/table.LogicalSectorSize) - cloudInitSectors
	table.Partitions = append(table.Partitions, &mbr.Partition{
		Bootable: false,
		Type:     mbr.Linux,
		Start:    cloudInitStart,
		Size:     cloudInitSectors,
	})

	// write partition table to disk
	log.Print("Writing partition table to disk")
	err = destDisk.Partition(table)
	if err != nil {
		log.Fatalf("Error writing partition table to disk %s: %v", diskPath, err)
	}

	log.Printf("Cleaning cloud init partition")
	b := make([]byte, destDisk.LogicalBlocksize*int64(cloudInitSectors))
	_, err = destDisk.WritePartitionContents(len(table.Partitions), bytes.NewReader(b))
	if err != nil {
		log.Fatalf("Error cleaning cloud-init partition: %v", err)
	}

	// create the cloud init filesystem
	log.Print("Creating cloud init filesystem")
	cloudInitFS, err := destDisk.CreateFilesystem(disk.FilesystemSpec{
		Partition:   len(table.Partitions),
		FSType:      filesystem.TypeFat32,
		VolumeLabel: "config-2",
	})
	if err != nil {
		log.Fatalf("Error creating cloud-init filesystem on %s: %v", diskPath, err)
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
