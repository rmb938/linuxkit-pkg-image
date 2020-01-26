package main

import (
	"bytes"
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

func zeroPartFix(mydisk *disk.Disk) {
	b := make([]byte, mydisk.LogicalBlocksize*2097152)
	_, err := mydisk.WritePartitionContents(1, bytes.NewReader(b))
	if err != nil {
		log.Fatalf("Error cleaning cloud-init partition: %v", err)
	}
}

func CreateFSAndDir(diskImg string) {
	if diskImg == "" {
		log.Fatal("must have a valid path for diskImg")
	}
	mydisk, err := diskfs.Open(diskImg)
	if err != nil {
		var diskSize int64
		diskSize = 10 * 1024 * 1024 // 10 MB
		mydisk, err = diskfs.Create(diskImg, diskSize, diskfs.Raw)
		check(err)
	}

	table := &mbr.Table{
		LogicalSectorSize:  512,
		PhysicalSectorSize: 512,
		Partitions: []*mbr.Partition{
			{
				Bootable: false,
				Type:     mbr.Linux,
				Start:    2048,
				Size:     2097152, // 1GB of space for the partition
			},
		},
	}

	log.Print("Writing partition table to disk")
	err = mydisk.Partition(table)
	check(err)

	// zeroPartFix(mydisk)

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
