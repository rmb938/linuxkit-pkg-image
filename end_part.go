package main

import (
	"fmt"
	"log"
	"os"
	"path"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/partition/mbr"
)

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func CreateFSAndDir(diskImg string) {
	if diskImg == "" {
		log.Fatal("must have a valid path for diskImg")
	}
	mydisk, err := diskfs.Open(diskImg)
	if err != nil {
		var diskSize int64
		diskSize = 5 * 1024 * 1024 * 1024 // 5 GB
		mydisk, err = diskfs.Create(diskImg, diskSize, diskfs.Raw)
		check(err)
	}

	cloudInitSize := 1 * 1024 * 1024 * 1024 // 1 GB
	cloudInitSectors := uint32(cloudInitSize / 512)
	// we want to create it at the end of the disk
	// so find the disk sector count and minus the cloudinit sectors
	cloudInitStart := uint32(int(mydisk.Size)/512) - cloudInitSectors

	table := &mbr.Table{
		LogicalSectorSize:  512,
		PhysicalSectorSize: 512,
		Partitions: []*mbr.Partition{
			{
				Bootable: false,
				Type:     mbr.Linux,
				Start:    cloudInitStart,
				Size:     cloudInitSectors, // 1GB of space for the partition
			},
		},
	}

	log.Print("Writing partition table to disk")
	err = mydisk.Partition(table)
	check(err)

	fspec := disk.FilesystemSpec{Partition: 1, FSType: filesystem.TypeFat32, VolumeLabel: "config-2"}
	fs, err := mydisk.CreateFilesystem(fspec)
	check(err)

	cloudInitPrefix := path.Join("/", "openstack", "latest")
	// place down cloud-init info
	log.Print("Creating cloud init directory structure")
	err = fs.Mkdir(cloudInitPrefix)
	if err != nil {
		log.Fatalf("Error creating cloud init directory structure: %v", err)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <outfile>\n", os.Args[0])
		os.Exit(1)
	}
	CreateFSAndDir(os.Args[1])
}
