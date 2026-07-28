package main

import (
	"log"

	"spacetime-node/internal/platform/app"
)

var version = "dev"

func main() {
	if err := app.Run("recommendation-service", ":8002", ":9002", version); err != nil {
		log.Fatal(err)
	}
}
