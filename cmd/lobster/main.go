// Package main provides the lobster command: crawl a site and check each page against performance targets.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"os"
	"time"

	"github.com/1mb-dev/lobster/v2/internal/cli"
	"github.com/1mb-dev/lobster/v2/internal/domain"
	"github.com/1mb-dev/lobster/v2/internal/load"
	"github.com/1mb-dev/lobster/v2/internal/reporter"
	"github.com/1mb-dev/lobster/v2/internal/tester"
	"github.com/1mb-dev/lobster/v2/internal/util"
	"github.com/1mb-dev/lobster/v2/internal/validator"
)

// Exit codes.
const (
	exitPass  = 0 // every enabled gate passed
	exitFail  = 1 // a target failed or the crawl was incomplete
	exitError = 2 // usage, configuration, refused base URL, or runtime error
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes one lobster invocation and returns its exit code.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lobster", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	var (
		configPath      = fs.String("config", "", "Path to configuration file (JSON)")
		allowPrivateIPs = fs.Bool("allow-private-ips", false, "Allow private/localhost IPs (for internal testing)")
		noProgress      = fs.Bool("no-progress", false, "Disable progress updates")
		showVersion     = fs.Bool("version", false, "Show version information")
		showHelp        = fs.Bool("help", false, "Show help message")
		iOwnThis        = fs.Bool("i-own-this", false, "Confirm you own the -load target (with LOBSTER_I_OWN_THIS=<host>)")
	)
	opts := cli.RegisterConfigFlags(fs)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			cli.ShowHelpMessage(stdout, displayVersion(currentVersion()))
			return exitPass
		}
		fmt.Fprintln(stderr, "Run 'lobster -help' for usage.")
		return exitError
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected argument %q; flags only. Run 'lobster -help' for usage.\n", fs.Arg(0))
		return exitError
	}

	var logLevel slog.LevelVar
	if opts.Verbose {
		logLevel.Set(slog.LevelDebug)
	}
	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: &logLevel}))

	if *showVersion {
		fmt.Fprintf(stdout, "Lobster %s\n", displayVersion(currentVersion()))
		return exitPass
	}
	if *showHelp {
		cli.ShowHelpMessage(stdout, displayVersion(currentVersion()))
		return exitPass
	}

	cfg, err := cli.LoadConfiguration(fs, *configPath, opts)
	if err != nil {
		logger.Error("Configuration error",
			"error", err,
			"hint", "Check your config file syntax or command-line flags")
		return exitError
	}
	if cfg.Verbose {
		logLevel.Set(slog.LevelDebug)
	}

	if cfg.UserAgent == "" {
		cfg.UserAgent = userAgent(currentVersion())
	}

	if confirmErr := cli.ConfirmLowRate(stderr, cfg.Rate); confirmErr != nil {
		logger.Error("Run canceled", "error", confirmErr)
		return exitError
	}

	if code := checkSafety(cfg, *allowPrivateIPs, stderr, logger); code != exitPass {
		return code
	}

	// Validate already parsed both durations.
	testDuration, _ := time.ParseDuration(cfg.Duration)
	requestTimeout, _ := time.ParseDuration(cfg.Timeout)

	testerConfig := domain.TesterConfig{
		BaseURL:            cfg.BaseURL,
		Concurrency:        cfg.Concurrency,
		RequestTimeout:     requestTimeout,
		UserAgent:          cfg.UserAgent,
		Auth:               cfg.Auth,
		MaxDepth:           cfg.MaxDepth,
		QueueSize:          cfg.QueueSize,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
		IgnoreRobots:       cfg.IgnoreRobots,
		AllowPrivateIPs:    *allowPrivateIPs,
		Rate:               cfg.Rate,
		Verbose:            cfg.Verbose,
		NoProgress:         *noProgress,
	}

	t, err := tester.New(testerConfig, logger)
	if err != nil {
		logger.Error("Run failed", "error", err)
		return exitError
	}

	// The ownership gate runs before the crawl: a refused load costs only the robots.txt fetch.
	var loadNeedsAck bool
	if cfg.Load != "" {
		if loadNeedsAck, err = checkLoadGate(cfg.BaseURL, t.CrawlDelay(), *iOwnThis); err != nil {
			logger.Error("Load refused", "error", err)
			return exitError
		}
	}

	results, err := runCrawl(t, testerConfig, testDuration, logger)
	if err != nil {
		logger.Error("Run failed", "error", err)
		return exitError
	}

	if cfg.Load != "" {
		if err = runLoad(t, cfg, results, !loadNeedsAck, stderr, logger); err != nil {
			logger.Error("Load failed", "error", err)
			return exitError
		}
	}

	performanceValidator := validator.New(cfg.PerformanceTargets)
	performanceValidator.ValidateResults(results)

	rep := reporter.New(results)
	rep.PrintSummary(stdout)
	performanceValidator.PrintValidationReport(stdout)

	results.PerformanceValidation = performanceValidator.GetValidationSummary()
	if cfg.OutputFile != "" {
		if err = rep.GenerateJSON(cfg.OutputFile); err != nil {
			logger.Error("Cannot write results",
				"file", cfg.OutputFile,
				"error", err,
				"hint", "Check file permissions and disk space")
			return exitError
		}
		logger.Info("Results saved", "file", cfg.OutputFile)
	}
	if cfg.HTMLFile != "" {
		if err = rep.GenerateHTML(cfg.HTMLFile); err != nil {
			logger.Error("Cannot write HTML report", "file", cfg.HTMLFile, "error", err)
			return exitError
		}
		logger.Info("HTML report saved", "file", cfg.HTMLFile)
	}

	if performanceValidator.Passed() {
		return exitPass
	}
	return exitFail
}

// checkSafety refuses an unsafe base URL or an unconfirmed TLS bypass, and
// prints the warning boxes for opted-out safety defaults.
func checkSafety(cfg *domain.Config, allowPrivateIPs bool, stderr io.Writer, logger *slog.Logger) int {
	if err := util.ValidateBaseURL(cfg.BaseURL, allowPrivateIPs); err != nil {
		logger.Error("Invalid base URL", "error", err)
		return exitError
	}

	if allowPrivateIPs {
		cli.PrintWarningBox(stderr, "SECURITY WARNING", []string{
			"Private/localhost IPs are ALLOWED (-allow-private-ips)",
			"",
			"This bypasses SSRF protection. Only use for:",
			"  • Testing internal/local services you own",
			"  • Development environments",
			"",
			"NEVER use this with untrusted URL inputs!",
		})
	}

	if cfg.InsecureSkipVerify {
		if os.Getenv("LOBSTER_INSECURE_TLS") != "true" {
			logger.Error("Insecure TLS requires explicit confirmation",
				"required_env", "LOBSTER_INSECURE_TLS=true",
				"reason", "security safeguard to prevent accidental TLS bypass",
				"implications", []string{
					"man-in-the-middle attacks become possible",
					"certificate validation is completely bypassed",
					"use only for testing with self-signed certificates",
				})
			return exitError
		}

		cli.PrintWarningBox(stderr, "SECURITY WARNING", []string{
			"TLS certificate verification is DISABLED (-insecure-skip-verify)",
			"",
			"This makes you vulnerable to man-in-the-middle attacks!",
			"",
			"Only use this for:",
			"  • Testing with self-signed certificates",
			"  • Internal development environments",
			"",
			"NEVER use this in production or with untrusted networks!",
		})
	}

	if cfg.IgnoreRobots {
		cli.PrintWarningBox(stderr, "ETHICAL WARNING", []string{
			"Ignoring robots.txt directives (-ignore-robots)",
			"",
			"You are bypassing the website owner's crawling preferences!",
			"",
			"Only use this when:",
			"  • You OWN the website being tested",
			"  • You have explicit written permission",
			"",
			"Unauthorized testing may violate terms of service or laws!",
		})
	}
	return exitPass
}

// runCrawl runs the crawl with -duration as its time ceiling.
func runCrawl(t *tester.Tester, config domain.TesterConfig, duration time.Duration, logger *slog.Logger) (*domain.TestResults, error) {
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	logger.Info("Starting crawl",
		"base_url", util.SanitizeURLDefault(config.BaseURL),
		"concurrency", config.Concurrency,
		"rate", config.Rate,
		"max_depth", config.MaxDepth)

	return t.Run(ctx)
}

// checkLoadGate refuses -load against a public host, or one whose robots.txt
// sets a Crawl-delay, unless -i-own-this is set and LOBSTER_I_OWN_THIS names
// the host. It reports whether the acknowledgement was needed: a load that did
// not need one must stay on private addresses (see runLoad).
func checkLoadGate(baseURL string, crawlDelay time.Duration, iOwnThis bool) (needed bool, err error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false, err
	}
	host := u.Hostname()
	var addrs []netip.Addr
	if addr, parseErr := netip.ParseAddr(host); parseErr == nil {
		addrs = []netip.Addr{addr}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		addrs, _ = net.DefaultResolver.LookupNetIP(ctx, "ip", host) // unresolved: NeedsAck says yes
	}
	needed = load.NeedsAck(addrs, crawlDelay)
	return needed, load.CheckAck(needed, iOwnThis, os.Getenv(load.AckEnv), host)
}

// runLoad replays the crawled URLs at -rate for -load, after a complete crawl
// only. With privateOnly, every connection must go to a private address, so a
// host that rebinds to a public one after the gate's check is refused.
func runLoad(t *tester.Tester, cfg *domain.Config, results *domain.TestResults, privateOnly bool, stderr io.Writer, logger *slog.Logger) error {
	if !results.CrawlComplete {
		logger.Warn("Load skipped: the crawl is incomplete")
		return nil
	}
	urls := make([]string, 0, len(results.URLValidations))
	for _, v := range results.URLValidations {
		if v.IsValid {
			urls = append(urls, v.URL)
		}
	}
	if len(urls) == 0 {
		logger.Warn("Load skipped: no crawled page responded 200-399")
		return nil
	}
	duration, _ := time.ParseDuration(cfg.Load) // checked by Validate

	cli.PrintWarningBox(stderr, "LOAD", []string{
		fmt.Sprintf("Sending %g req/s to %s for %s", cfg.Rate, util.SanitizeURLDefault(cfg.BaseURL), duration),
		fmt.Sprintf("%d pages that responded 200-399, up to %d requests in flight", len(urls), load.InFlight),
		fmt.Sprintf("Stops after %d consecutive 429/5xx responses", load.AbortAfter),
	})

	loadResults, err := load.Run(context.Background(), load.Config{
		Client:    t.LoadClient(load.InFlight, privateOnly),
		Requester: t,
		URLs:      urls,
		Rate:      cfg.Rate,
		Duration:  duration,
	})
	if err != nil {
		return err
	}
	results.Load = loadResults
	return nil
}
