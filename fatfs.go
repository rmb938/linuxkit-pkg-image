package main

import (
	"fmt"
	"log"
	"os"

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
func CreateFS(diskImg string) {
	if diskImg == "" {
		log.Fatal("must have a valid path for diskImg")
	}
	var diskSize int64
	diskSize = 10 * 1024 * 1024 * 1024 // 10 GB
	mydisk, err := diskfs.Create(diskImg, diskSize, diskfs.Raw)
	check(err)

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

	fspec := disk.FilesystemSpec{Partition: 1, FSType: filesystem.TypeFat32, VolumeLabel: "config-2"}
	fs, err := mydisk.CreateFilesystem(fspec)
	check(err)
	fmt.Printf("%#v\n", fs)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <outfile>\n", os.Args[0])
		os.Exit(1)
	}
	CreateFS(os.Args[1])
}
