package main

import "os"

func printUsage() {
	println("Usage: exif-geotagger <command> [options]")
	println()
	println("Commands:")
	println("  build-db      Extract GPS data from reference images and build database")
	println("  print-db      Print database contents as JSON")
	println("  tag-images    Tag raw images with GPS data from database")
	println()
	println("Run 'exif-geotagger <command> -h' for more information on a command.")
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "build-db":
		runBuildDB()
	case "print-db":
		runPrintDB()
	case "tag-images":
		runTagImages()
	default:
		printUsage()
		os.Exit(1)
	}
}
