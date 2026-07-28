package main

import (
	"log"

	"spacetime-node/internal/platform/app"
)

var version = "dev"

func main() {
	if err := app.Run("mobility-service", ":8001", ":9001", version); err != nil {
		log.Fatal(err)
	}
}
