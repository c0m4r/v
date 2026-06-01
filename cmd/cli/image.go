package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/c0m4r/v/engine"
)

func cmdImage(e *engine.Engine, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: v image <pull|list>")
	}

	switch args[0] {
	case "pull":
		return cmdImagePull(e, args[1:])
	case "list", "ls":
		return cmdImageList(e)
	case "available":
		return cmdImageAvailable(e)
	default:
		return fmt.Errorf("unknown image command: %s", args[0])
	}
}

func cmdImagePull(e *engine.Engine, args []string) error {
	if len(args) == 0 {
		fmt.Println("Available images:")
		for name := range e.KnownImages() {
			fmt.Printf("  %s\n", name)
		}
		return fmt.Errorf("usage: v image pull <name|url>")
	}

	name := args[0]
	fmt.Printf("Pulling image %q...\n", name)

	path, err := e.PullImage(name, func(bytes, total int64) {
		const mib = 1024.0 * 1024.0
		got := float64(bytes) / mib
		if total > 0 {
			tot := float64(total) / mib
			pct := float64(bytes) * 100 / float64(total)
			fmt.Fprintf(os.Stderr, "\rDownloaded: %7.1f / %7.1f MB (%5.1f%%)", got, tot, pct)
		} else {
			fmt.Fprintf(os.Stderr, "\rDownloaded: %7.1f MB", got)
		}
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "\n")
	fmt.Printf("Image cached at: %s\n", path)
	return nil
}

func cmdImageList(e *engine.Engine) error {
	images, err := e.ListImages()
	if err != nil {
		return err
	}

	if len(images) == 0 {
		fmt.Println("No cached images. Pull one with: v image pull <name>")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tSIZE")
	for _, img := range images {
		_, _ = fmt.Fprintf(w, "%s\t%.1f MB\n", img.Name, float64(img.Size)/1024/1024)
	}
	return w.Flush()
}

func cmdImageAvailable(e *engine.Engine) error {
	fmt.Println("Available cloud images:")
	for name, url := range e.KnownImages() {
		fmt.Printf("  %-16s %s\n", name, url)
	}
	return nil
}
