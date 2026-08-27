// Command build renders data/ + web/ into dist/.
package main

import (
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"

	"portfolio/internal/render"
)

const description = "Sarbojit Rana — backend engineer. Go services, Postgres, Redis, and command-line tools."

func main() {
	root := flag.String("root", ".", "repository root")
	out := flag.String("out", "dist", "output directory")
	flag.Parse()

	site := render.Site{Root: *root}
	frag, err := site.Render()
	if err != nil {
		log.Fatalf("render: %v", err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	write(filepath.Join(*out, "index.html"), render.Document(frag, description))
	copyFile(filepath.Join(*root, "web", "style.css"), filepath.Join(*out, "style.css"))
	copyFile(filepath.Join(*root, "web", "app.js"), filepath.Join(*out, "app.js"))
	copyTree(filepath.Join(*root, "assets"), filepath.Join(*out, "assets"))

	inline := render.Site{Root: *root, Inline: true}
	fragInline, err := inline.Render()
	if err != nil {
		log.Fatalf("render inline: %v", err)
	}
	write(filepath.Join(*out, "artifact.html"), fragInline)

	log.Printf("wrote %s (%d kB) and artifact.html (%d kB)",
		filepath.Join(*out, "index.html"), len(frag)/1024, len(fragInline)/1024)
}

func write(path string, b []byte) {
	if err := os.WriteFile(path, b, 0o644); err != nil {
		log.Fatal(err)
	}
}

func copyFile(src, dst string) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		log.Fatal(err)
	}
	in, err := os.Open(src)
	if err != nil {
		log.Fatal(err)
	}
	defer in.Close()
	f, err := os.Create(dst)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if _, err := io.Copy(f, in); err != nil {
		log.Fatal(err)
	}
}

func copyTree(src, dst string) {
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		copyFile(p, filepath.Join(dst, rel))
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		log.Fatal(err)
	}
}
