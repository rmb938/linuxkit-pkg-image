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

func CreateFSAndDir(diskImg string) {
	if diskImg == "" {
		log.Fatal("must have a valid path for diskImg")
	}
	mydisk, err := diskfs.Open(diskImg)
	check(err)

	cloudInitSize := 10 * 1024 * 1024 // 10 MB
	cloudInitSectors := uint32(cloudInitSize / 512)

	table := &mbr.Table{
		LogicalSectorSize:  512,
		PhysicalSectorSize: 512,
		Partitions: []*mbr.Partition{
			{
				Bootable: false,
				Type:     mbr.Linux,
				Start:    2048,
				Size:     cloudInitSectors,
			},
		},
	}

	log.Print("Writing partition table to disk")
	err = mydisk.Partition(table)
	check(err)

	fspec := disk.FilesystemSpec{Partition: 1, FSType: filesystem.TypeFat32, VolumeLabel: "config-2"}
	_, err := mydisk.CreateFilesystem(fspec)
	check(err)

}

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <outfile>\n", os.Args[0])
		os.Exit(1)
	}
	CreateFSAndDir(os.Args[1])
}
