// Command hanji is the Hanji compiler and development server.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"hanji"
	"hanji/internal/server"
	"hanji/internal/site"
)

const usage = `Hanji %s — a markdown-first web framework where the document is the app

Usage:
  hanji new <dir>            create a new site
  hanji dev [dir]            start the dev server (reloads on save)
  hanji build [dir]          build the static site into dist/
  hanji preview [dir]        serve the built site locally
  hanji version

Options:
  dev, preview:  --port 5151   --host localhost
  build:         --out dist
`

func main() {
	if len(os.Args) < 2 {
		fmt.Printf(usage, site.Version)
		return
	}
	cmd, args := os.Args[1], os.Args[2:]
	fl := flag.NewFlagSet(cmd, flag.ExitOnError)
	port := fl.Int("port", 5151, "port to listen on")
	host := fl.String("host", "localhost", "host to listen on")
	out := fl.String("out", "", "output directory (default <dir>/dist)")
	// allow flags before or after the directory argument
	var dirArg string
	for len(args) > 0 {
		fl.Parse(args)
		args = fl.Args()
		if len(args) > 0 {
			dirArg, args = args[0], args[1:]
		}
	}
	dir := "."
	if dirArg != "" {
		dir = dirArg
	}
	addr := fmt.Sprintf("%s:%d", *host, *port)
	if *out == "" {
		*out = filepath.Join(dir, "dist")
	}

	var err error
	switch cmd {
	case "new":
		if dirArg == "" {
			err = fmt.Errorf("where should the site go? e.g. hanji new my-site")
			break
		}
		err = scaffold(dirArg)
	case "dev":
		err = server.Dev(site.Options{Dir: dir, Theme: hanji.Theme(), Dev: true}, addr)
	case "build":
		start := time.Now()
		var res *site.Result
		if res, err = site.Build(site.Options{Dir: dir, Theme: hanji.Theme()}); err == nil {
			if err = res.Write(*out); err == nil {
				fmt.Printf("✓ %d pages, %d files → %s (%s)\n", res.Pages, len(res.Files), *out, time.Since(start).Round(time.Millisecond))
			}
		}
	case "preview":
		err = server.Preview(*out, addr)
	case "version", "--version", "-v":
		fmt.Println("hanji", site.Version)
	case "help", "--help", "-h":
		fmt.Printf(usage, site.Version)
	default:
		err = fmt.Errorf("unknown command: %s (try `hanji help`)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hanji:", err)
		os.Exit(1)
	}
}

// scaffold writes the starter project into dir, which must be new or empty.
func scaffold(dir string) error {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s is not empty", dir)
	}
	starter := hanji.Starter()
	err := fs.WalkDir(starter, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := fs.ReadFile(starter, p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		return err
	}
	fmt.Printf("✓ created a new site in %s\n\n  cd %s\n  hanji dev\n\n", dir, dir)
	return nil
}
