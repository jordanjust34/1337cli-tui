// Package core holds the operations both binaries share: searching 1337x,
// listing qBittorrent destinations, and resolving and submitting torrents.
// Front ends own parsing input and presenting output; nothing here writes to
// stdout or stderr.
package core

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"1337cli/internal/qbit"
	"1337cli/internal/scraper"
)

// Aliases so front ends only need to import core.
type (
	Result     = scraper.Result
	Category   = qbit.Category
	AddOptions = qbit.AddOptions
)

// ErrNoQbit is returned by qBittorrent operations when no server URL is configured.
var ErrNoQbit = errors.New("no qBittorrent server configured")

// Service runs the shared operations with one Config.
type Service struct {
	Config Config
	// Warn receives non-fatal problems, e.g. a later search page failing. Nil drops them.
	Warn func(msg string)

	scraper *scraper.Client
}

func New(cfg Config) *Service {
	sc := scraper.NewClient(cfg.Mirror)
	sc.FlareSolver = cfg.FlareSolverr
	sc.Cookie = cfg.Cookie
	if cfg.UserAgent != "" {
		sc.UserAgent = cfg.UserAgent
	}
	return &Service{Config: cfg, scraper: sc}
}

func (s *Service) warnf(format string, args ...any) {
	if s.Warn != nil {
		s.Warn(fmt.Sprintf(format, args...))
	}
}

// Explain adds remediation hints to a Cloudflare challenge error.
func (s *Service) Explain(err error) error {
	if errors.Is(err, scraper.ErrChallenge) {
		return fmt.Errorf("%w\n\n%s fronts the site with a Cloudflare challenge. Either:\n"+
			"  - run FlareSolverr and set FLARESOLVERR_URL=http://localhost:8191 (1337cli: --flaresolverr), or\n"+
			"  - set LEET_COOKIE='cf_clearance=...' and LEET_USER_AGENT copied from a browser that passed the check (1337cli: --cookie, --user-agent)", err, s.Config.Mirror)
	}
	return err
}

// pageSize is how many entries 1337x shows per search page; a shorter page is the last one.
const pageSize = 20

// Search fetches results starting at page until it has limit entries or runs
// out of pages. A limit of 0 means a single page. Challenge errors come back
// with remediation hints.
func (s *Service) Search(query string, page, limit int) ([]Result, error) {
	if page < 1 {
		return nil, errors.New("page must be 1 or greater")
	}
	if limit < 0 {
		return nil, errors.New("limit must be 0 or greater")
	}
	var all []Result
	for p := page; ; p++ {
		results, err := s.scraper.Search(query, p)
		// 1337x occasionally serves an empty table mid-pagination; retry once
		// before treating an empty later page as the end of the results.
		if err == nil && len(results) == 0 && len(all) > 0 {
			results, err = s.scraper.Search(query, p)
			if err == nil && len(results) == 0 {
				s.warnf("page %d returned no results; stopping at %d entries", p, len(all))
			}
		}
		if err != nil {
			// Keep what we already have if a later page fails.
			if len(all) > 0 {
				s.warnf("stopped at page %d: %v", p, err)
				break
			}
			return nil, s.Explain(err)
		}
		all = append(all, results...)
		if limit == 0 || len(all) >= limit || len(results) < pageSize {
			break
		}
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func (s *Service) qbit() (*qbit.Client, error) {
	if s.Config.QbitURL == "" {
		return nil, fmt.Errorf("%w: set QBIT_URL in the environment or %s", ErrNoQbit, ConfigPath())
	}
	return qbit.NewClient(s.Config.QbitURL, s.Config.QbitUser, s.Config.QbitPass), nil
}

// Categories lists the qBittorrent categories, sorted by name.
func (s *Service) Categories() ([]Category, error) {
	qc, err := s.qbit()
	if err != nil {
		return nil, err
	}
	return qc.Categories()
}

// Outcome is what happened to one submitted target.
type Outcome struct {
	Target string // page URL or magnet as given
	Name   string // the magnet's display name, or Target if it has none
	Err    error  // nil if qBittorrent accepted it
}

// Submit resolves each target to a magnet link (fetching 1337x pages as
// needed) and adds it to qBittorrent with opts. It keeps going past individual
// failures, which are reported in the outcomes; the error is only for problems
// that stop the whole batch, such as a missing server or failed login.
// progress, if not nil, is called after each target.
func (s *Service) Submit(targets []string, opts AddOptions, progress func(Outcome)) ([]Outcome, error) {
	qc, err := s.qbit()
	if err != nil {
		return nil, err
	}
	if err := qc.Login(); err != nil {
		return nil, err
	}
	outs := make([]Outcome, 0, len(targets))
	for _, t := range targets {
		o := s.submitOne(qc, t, opts)
		outs = append(outs, o)
		if progress != nil {
			progress(o)
		}
	}
	return outs, nil
}

func (s *Service) submitOne(qc *qbit.Client, target string, opts AddOptions) Outcome {
	magnet := target
	if !strings.HasPrefix(target, "magnet:") {
		m, err := s.scraper.Magnet(target)
		if err != nil {
			return Outcome{Target: target, Name: target, Err: s.Explain(err)}
		}
		magnet = m
	}
	return Outcome{Target: target, Name: magnetName(magnet, target), Err: qc.AddMagnet(magnet, opts)}
}

// Failed counts the outcomes that weren't accepted.
func Failed(outs []Outcome) int {
	n := 0
	for _, o := range outs {
		if o.Err != nil {
			n++
		}
	}
	return n
}

// magnetName returns the magnet's display name, falling back to the target.
func magnetName(magnet, fallback string) string {
	if i := strings.Index(magnet, "?"); i >= 0 {
		if q, err := url.ParseQuery(magnet[i+1:]); err == nil && q.Get("dn") != "" {
			return q.Get("dn")
		}
	}
	return fallback
}
