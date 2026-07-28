package main

import (
	"log"

	"spacetime-node/internal/platform/app"
)

var version = "dev"

func main() {
	if err := app.Run("gateway-service", ":8000", ":9000", version); err != nil {
		log.Fatal(err)
	}
}
