// Package mail is the real, SMTP-backed mailer design.md's file table
// (line 473) mandates: a go-mail SMTP sender behind a bounded async
// dispatch pool, plus the two html/template templates D-N names
// (password_reset.html, login_lockout.html; see templates.go).
//
// internal/platform/mail.LogMailer remains the interim, log-only
// stand-in used by tests and by any deployment that deliberately has no
// SMTP; nothing in this codebase falls back to it automatically -- the
// only way to get a LogMailer is to construct one explicitly in source,
// exactly as the tests already do. cmd/vecingest/serve.go wires
// *AsyncMailer here unconditionally, because SMTP_URL and MAIL_FROM are
// already `always`-required for the serve command (design D-I): they
// must be used, not merely validated and then ignored.
package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"sync"
	"time"

	gomail "github.com/wneessen/go-mail"
)

// Mailer is internal/mail's own domain-facing port (design.md:473).
// handlers.RawSender declares the identical method shape, so *AsyncMailer
// and internal/platform/mail.LogMailer both satisfy it structurally,
// with no adapter type required.
type Mailer interface {
	SendRaw(ctx context.Context, to, subject, body string) error
}

const (
	// defaultPoolSize is the bounded worker pool's fixed size (design
	// D-N: "bounded worker pool ... fixed size").
	defaultPoolSize = 4
	// defaultQueueSize is the capped queue's capacity (design D-N:
	// "capped queue"): SendRaw drops rather than blocks once it's full.
	defaultQueueSize = 64
	// defaultSendTimeout bounds a single SMTP dial+send inside a
	// worker, so one dead server can never wedge a worker forever.
	defaultSendTimeout = 10 * time.Second
)

// Config configures the real SMTP-backed AsyncMailer.
type Config struct {
	// SMTPURL is SMTP_URL (config.Holder.SMTPURL()):
	// "smtp://[user:pass@]host:port" for STARTTLS-opportunistic
	// delivery, or "smtps://[user:pass@]host:port" for implicit TLS.
	SMTPURL string
	// From is MAIL_FROM (config.Config.MailFrom), an RFC 5322 mailbox,
	// e.g. "Vecingest <no-reply@example.com>".
	From string
	// PoolSize is the number of worker goroutines draining the send
	// queue. Defaults to 4 when <= 0.
	PoolSize int
	// QueueSize caps how many pending sends may be buffered before
	// SendRaw drops one instead of blocking its caller. Defaults to 64
	// when <= 0.
	QueueSize int
	// SendTimeout bounds a single SMTP dial+send. Defaults to 10s when
	// <= 0.
	SendTimeout time.Duration
	// Logger defaults to slog.Default() when nil.
	Logger *slog.Logger

	// dialContextFunc overrides go-mail's own dialer. Test-only seam:
	// production wiring never sets it, so production always makes a
	// real TCP (or TLS) connection; tests use it to inject a fake SMTP
	// transport with no real network call.
	dialContextFunc gomail.DialContextFunc
}

type sendJob struct {
	to, subject, body string
}

// AsyncMailer is the real, SMTP-backed Mailer (design.md:473, D-N "the
// alert"). A bounded worker pool drains a capped queue so a slow or
// dead SMTP server can never block the request path that called
// SendRaw -- lockout.Service and ForgotPasswordService already dispatch
// off their own call stack via a goroutine before ever reaching here
// (auth-credentials: Progressive Lockout's alert must never become a
// response-latency timing oracle for account existence), so this pool
// is a second, independent, defense-in-depth guarantee of the same
// property: even a caller that invoked SendRaw synchronously from a
// request handler would still never block on SMTP. Close drains the
// pool on graceful shutdown instead of dropping in-flight mail when the
// process exits.
type AsyncMailer struct {
	client      *gomail.Client
	from        string
	jobs        chan sendJob
	sendTimeout time.Duration
	logger      *slog.Logger
	wg          sync.WaitGroup
	closeOnce   sync.Once
}

// New builds an AsyncMailer from cfg: it parses SMTP_URL and starts
// cfg.PoolSize worker goroutines. It performs no network I/O itself --
// dialing happens per send, inside a worker -- so a misconfigured or
// unreachable SMTP server fails individual sends rather than New.
func New(cfg Config) (*AsyncMailer, error) {
	if cfg.From == "" {
		return nil, errors.New("mail: From is required")
	}

	host, opts, err := parseSMTPURL(cfg.SMTPURL)
	if err != nil {
		return nil, fmt.Errorf("mail: %w", err)
	}
	if cfg.dialContextFunc != nil {
		opts = append(opts, gomail.WithDialContextFunc(cfg.dialContextFunc))
	}
	client, err := gomail.NewClient(host, opts...)
	if err != nil {
		return nil, fmt.Errorf("mail: build SMTP client: %w", err)
	}

	poolSize := cfg.PoolSize
	if poolSize <= 0 {
		poolSize = defaultPoolSize
	}
	queueSize := cfg.QueueSize
	if queueSize <= 0 {
		queueSize = defaultQueueSize
	}
	sendTimeout := cfg.SendTimeout
	if sendTimeout <= 0 {
		sendTimeout = defaultSendTimeout
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	m := &AsyncMailer{
		client:      client,
		from:        cfg.From,
		jobs:        make(chan sendJob, queueSize),
		sendTimeout: sendTimeout,
		logger:      logger,
	}
	m.wg.Add(poolSize)
	for range poolSize {
		go m.worker()
	}
	return m, nil
}

// SendRaw enqueues an email for asynchronous delivery and never blocks:
// when the bounded queue is full, the message is dropped and logged
// rather than backing up onto the caller. Never logs to (PII, D-J).
func (m *AsyncMailer) SendRaw(ctx context.Context, to, subject, body string) error {
	select {
	case m.jobs <- sendJob{to: to, subject: subject, body: body}:
		return nil
	default:
		m.logger.WarnContext(ctx, "mail: send queue full, dropping message", "subject", subject)
		return errors.New("mail: send queue full")
	}
}

func (m *AsyncMailer) worker() {
	defer m.wg.Done()
	for job := range m.jobs {
		m.send(job)
	}
}

func (m *AsyncMailer) send(job sendJob) {
	msg := gomail.NewMsg()
	if err := msg.From(m.from); err != nil {
		m.logger.Error("mail: invalid From address", "error", err)
		return
	}
	if err := msg.To(job.to); err != nil {
		// job.to itself is never logged -- PII (D-J).
		m.logger.Error("mail: invalid To address")
		return
	}
	msg.Subject(job.subject)
	msg.SetBodyString(gomail.TypeTextHTML, job.body)

	ctx, cancel := context.WithTimeout(context.Background(), m.sendTimeout)
	defer cancel()
	if err := m.client.DialAndSendWithContext(ctx, msg); err != nil {
		m.logger.Error("mail: send failed", "error", err)
	}
}

// Close stops accepting new sends and waits for the worker pool to
// drain the queue (design D-N: "drained on graceful shutdown"), or
// until ctx is done, whichever comes first. Safe to call more than
// once; only the first call has effect.
func (m *AsyncMailer) Close(ctx context.Context) error {
	m.closeOnce.Do(func() { close(m.jobs) })
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// parseSMTPURL parses SMTP_URL into a go-mail host + Option list.
// "smtp://" gets opportunistic STARTTLS (falls back to plaintext if the
// server doesn't advertise it -- needed for local dev SMTP catchers
// that speak no TLS at all); "smtps://" gets implicit TLS on connect.
func parseSMTPURL(raw string) (host string, opts []gomail.Option, err error) {
	if raw == "" {
		return "", nil, errors.New("SMTP_URL is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", nil, fmt.Errorf("invalid SMTP_URL: %w", err)
	}
	// A fixed HELO identity, rather than go-mail's os.Hostname()
	// default: a container's actual hostname is often a random ID with
	// no dot, which some SMTP servers (rightly) refuse as an invalid
	// HELO/EHLO argument (RFC 5321 §4.1.4).
	//
	// WithoutRset: go-mail otherwise issues one RSET per message after
	// a successful delivery, purely as pre-emptive transaction-state
	// hygiene before the next Send call on the same connection; this
	// mailer never reuses a connection across sends (one dial per
	// job), so it's a wasted round trip, and some SMTP servers handle
	// a post-delivery RSET poorly.
	opts = append(opts, gomail.WithHELO("vecingest.invalid"), gomail.WithoutRset())

	switch u.Scheme {
	case "smtp":
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSOpportunistic))
	case "smtps":
		opts = append(opts, gomail.WithSSL())
	default:
		return "", nil, fmt.Errorf("invalid SMTP_URL scheme %q: must be smtp or smtps", u.Scheme)
	}
	host = u.Hostname()
	if host == "" {
		return "", nil, errors.New("invalid SMTP_URL: missing host")
	}
	if portStr := u.Port(); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return "", nil, fmt.Errorf("invalid SMTP_URL port %q: %w", portStr, err)
		}
		opts = append(opts, gomail.WithPort(port))
	}
	if u.User != nil {
		username := u.User.Username()
		password, hasPassword := u.User.Password()
		if username != "" && hasPassword {
			opts = append(opts,
				gomail.WithSMTPAuth(gomail.SMTPAuthAutoDiscover),
				gomail.WithUsername(username),
				gomail.WithPassword(password),
			)
		}
	}
	return host, opts, nil
}
