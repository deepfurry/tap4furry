// Preflight inspects prepared DB identity/capabilities without applying changes.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/config"
	"github.com/deepfurry/tap4furry/server/internal/database"
)

func main() {
	service := flag.String("service", "migrator", "prepared service role")
	flag.Parse()
	if err := run(*service); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(service string) error {
	c, err := config.Load(service)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, c.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	capabilities, err := database.Inspect(ctx, pool, "gfp_"+service, "gfp_dev")
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(capabilities)
}
