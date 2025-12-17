package main

import (
	"flag"
	"fmt"
	"log"

	root "github.com/risingwavelabs/events-api"
	"github.com/risingwavelabs/events-api/wire"
)

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println(root.Version)
		return
	}

	app, err := wire.InitApp()
	if err != nil {
		log.Fatal(err)
	}
	defer app.Shutdown()

	if err := app.Listen(); err != nil {
		log.Fatal(err)
	}
	log.Println("bye.")
}
