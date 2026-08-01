package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"freedeepseek-go/proxy"
)

func main() {
	portFlag := flag.String("port", "9655", "Port to run proxy server on")
	flag.Parse()

	// Get directory of execution
	ex, err := os.Executable()
	baseDir := "."
	if err == nil {
		baseDir = filepath.Dir(ex)
	}
	if cwd, err := os.Getwd(); err == nil {
		baseDir = cwd
	}

	server, err := proxy.NewServer(baseDir)
	if err != nil {
		log.Fatalf("Fatal error initializing FreeDeepseek-Go proxy: %v", err)
	}

	if err := server.Start(*portFlag); err != nil {
		log.Fatalf("Server shutdown with error: %v", err)
	}
}
