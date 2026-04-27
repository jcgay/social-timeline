// Command social-timeline aggregates Bluesky and Mastodon home
// timelines for a given window into a Markdown document on stdout.
//
// See docs/superpowers/specs/2026-04-26-social-timeline-design.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/jcgay/social-timeline/internal/bluesky"
	"github.com/jcgay/social-timeline/internal/mastodon"
	"github.com/jcgay/social-timeline/internal/render"
	"github.com/jcgay/social-timeline/internal/timeline"
	"github.com/jcgay/social-timeline/internal/timeparse"
)

const blueskyDefaultBaseURL = "https://bsky.social"

var version = "dev"

type runOpts struct {
	since    time.Time
	maxPosts int
	clients  []timeline.Client
	stdout   io.Writer
	stderr   io.Writer
	verbose  bool
}

func main() {
	os.Exit(realMain())
}

func realMain() int {
	fs := flag.NewFlagSet("social-timeline", flag.ContinueOnError)
	var (
		sinceStr  = fs.String("since", "", "Required. Duration (1d, 6h, 30m, 2w) or ISO-8601 date.")
		output    = fs.String("output", "", "Output file (default: stdout)")
		maxPosts  = fs.Int("max", 0, "Max posts per platform (0 = no limit)")
		platforms = fs.String("platform", "", "Comma-separated platforms (bluesky,mastodon). Default: every configured platform.")
		verbose   = fs.Bool("verbose", false, "Verbose logging on stderr")
		showVer   = fs.Bool("version", false, "Print version and exit")
	)
	fs.StringVar(output, "o", "", "Output file (default: stdout) (shorthand)")
	fs.BoolVar(verbose, "v", false, "Verbose logging on stderr (shorthand)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return 1
	}
	if *showVer {
		_, _ = fmt.Fprintln(os.Stdout, version)
		return 0
	}
	if *sinceStr == "" {
		fmt.Fprintln(os.Stderr, "error: --since is required")
		fs.SetOutput(os.Stderr)
		fs.Usage()
		return 1
	}
	since, err := timeparse.ParseSince(*sinceStr, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	clients, err := buildClients(*platforms, *verbose, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if len(clients) == 0 {
		fmt.Fprintln(os.Stderr, "error: no platform configured (set BLUESKY_* and/or MASTODON_* env vars)")
		return 1
	}

	var stdout io.Writer = os.Stdout
	var outputFile *os.File
	if *output != "" {
		outputFile, err = os.Create(*output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: open %s: %v\n", *output, err)
			return 1
		}
		defer func() { _ = outputFile.Close() }() // safety net for error paths
		stdout = outputFile
	}

	code := run(context.Background(), runOpts{
		since:    since,
		maxPosts: *maxPosts,
		clients:  clients,
		stdout:   stdout,
		stderr:   os.Stderr,
		verbose:  *verbose,
	})

	if outputFile != nil {
		if closeErr := outputFile.Close(); closeErr != nil && code == 0 {
			fmt.Fprintf(os.Stderr, "error: close %s: %v\n", *output, closeErr)
			code = 1
		}
		if code == 1 {
			_ = os.Remove(*output)
		}
	}
	return code
}

// buildClients reads env vars and constructs the clients for every
// configured platform, optionally restricted by the --platform flag.
func buildClients(platformFilter string, verbose bool, stderr io.Writer) ([]timeline.Client, error) {
	wanted := map[string]bool{}
	if platformFilter != "" {
		for _, p := range strings.Split(platformFilter, ",") {
			p = strings.TrimSpace(strings.ToLower(p))
			switch p {
			case "bluesky", "mastodon":
				wanted[p] = true
			default:
				return nil, fmt.Errorf("--platform: unknown %q (allowed: bluesky, mastodon)", p)
			}
		}
	}

	var clients []timeline.Client

	bskyHandle := os.Getenv("BLUESKY_HANDLE")
	bskyPwd := os.Getenv("BLUESKY_APP_PASSWORD")
	if (len(wanted) == 0 || wanted["bluesky"]) && bskyHandle != "" && bskyPwd != "" {
		clients = append(clients, bluesky.New(blueskyDefaultBaseURL, bskyHandle, bskyPwd))
	} else if (len(wanted) == 0 || wanted["bluesky"]) && verbose {
		_, _ = fmt.Fprintln(stderr, "skipping bluesky: BLUESKY_HANDLE / BLUESKY_APP_PASSWORD not set")
	}

	mastoURL := os.Getenv("MASTODON_INSTANCE_URL")
	mastoTok := os.Getenv("MASTODON_ACCESS_TOKEN")
	if (len(wanted) == 0 || wanted["mastodon"]) && mastoURL != "" && mastoTok != "" {
		clients = append(clients, mastodon.New(mastoURL, mastoTok))
	} else if (len(wanted) == 0 || wanted["mastodon"]) && verbose {
		_, _ = fmt.Fprintln(stderr, "skipping mastodon: MASTODON_INSTANCE_URL / MASTODON_ACCESS_TOKEN not set")
	}

	return clients, nil
}

// run is the testable core: it fans out to clients, merges results,
// and writes Markdown. Returns the process exit code.
func run(ctx context.Context, opts runOpts) int {
	type result struct {
		name  string
		posts []timeline.Post
		err   error
	}
	results := make([]result, len(opts.clients))

	g, gctx := errgroup.WithContext(ctx)
	for i, c := range opts.clients {
		i, c := i, c
		g.Go(func() error {
			posts, err := c.FetchHomeTimeline(gctx, opts.since, opts.maxPosts)
			results[i] = result{name: c.Name(), posts: posts, err: err}
			return nil // never propagate; we want all clients to run
		})
	}
	_ = g.Wait()

	var (
		all      []timeline.Post
		failures []string
	)
	successes := 0
	for _, r := range results {
		if r.err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", r.name, r.err))
			continue
		}
		successes++
		all = append(all, r.posts...)
	}
	for _, f := range failures {
		_, _ = fmt.Fprintln(opts.stderr, "warning:", f)
	}
	if successes == 0 {
		return 1
	}
	merged := timeline.MergeSort(all)
	if err := render.Markdown(merged, opts.stdout); err != nil {
		_, _ = fmt.Fprintf(opts.stderr, "error: write markdown: %v\n", err)
		return 1
	}
	if len(failures) > 0 {
		return 2
	}
	return 0
}
