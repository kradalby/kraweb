package kraweb

import (
	"crypto/tls"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"tailscale.com/tsnet"
	"tailscale.com/tsweb"
)

type KraWeb struct {
	// hostname is the name that will be used when joining Tailscale
	hostname string

	tsKeyPath  string
	controlURL string
	verbose    bool
	localAddr  string
	logger     *log.Logger

	mux   *http.ServeMux
	tsmux *http.ServeMux
}

func NewKraWeb(
	hostname string,
	tsKeyPath string,
	controlURL string,
	verbose bool,
	localAddr string,
	logger *log.Logger,
) KraWeb {
	k := KraWeb{
		hostname:   hostname,
		tsKeyPath:  tsKeyPath,
		controlURL: controlURL,
		verbose:    verbose,
		localAddr:  localAddr,
		logger:     logger,
	}
	k.mux = http.NewServeMux()
	k.tsmux = http.NewServeMux()

	return k
}

func (k *KraWeb) Handle(pattern string, handler http.Handler) {
	k.mux.Handle(pattern, handler)
	k.tsmux.Handle(pattern, handler)
}

func (k *KraWeb) HandleTSOnly(pattern string, handler http.Handler) {
	k.tsmux.Handle(pattern, handler)
}

func (k *KraWeb) ListenAndServe() error {
	tsweb.Debugger(k.tsmux)

	k.logger.SetPrefix("kraweb: ")
	log := k.logger

	tsSrv := &tsnet.Server{
		Hostname:   k.hostname,
		Logf:       func(format string, args ...any) {},
		ControlURL: k.controlURL,
	}

	if k.tsKeyPath != "" {
		key, err := os.ReadFile(k.tsKeyPath)
		if err != nil {
			return err
		}

		tsSrv.AuthKey = strings.TrimSuffix(string(key), "\n")
	}

	if k.verbose {
		tsSrv.Logf = log.Printf
	}

	if err := tsSrv.Start(); err != nil {
		return err
	}

	localClient, _ := tsSrv.LocalClient()

	k.tsmux.Handle("/metrics", promhttp.Handler())
	k.tsmux.Handle("/who", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		who, err := localClient.WhoIs(r.Context(), r.RemoteAddr)
		if err != nil {
			http.Error(w, err.Error(), 500)

			return
		}
		fmt.Fprintf(w, "<html><body><h1>Hello, world!</h1>\n")
		fmt.Fprintf(w, "<p>You are <b>%s</b> from <b>%s</b> (%s)</p>",
			html.EscapeString(who.UserProfile.LoginName),
			html.EscapeString(firstLabel(who.Node.ComputedName)),
			r.RemoteAddr)
	}))

	httpSrv := &http.Server{
		Handler:     k.mux,
		ErrorLog:    k.logger,
		ReadTimeout: 5 * time.Minute,
	}

	tshttpSrv := &http.Server{
		Handler:     k.tsmux,
		ErrorLog:    k.logger,
		ReadTimeout: 5 * time.Minute,
	}

	// Starting HTTPS server
	go func() {
		ts443, err := tsSrv.Listen("tcp", ":443")
		if err != nil {
			log.Printf("failed to start https server: %s", err)
		}
		ts443 = tls.NewListener(ts443, &tls.Config{
			GetCertificate: localClient.GetCertificate,
		})

		log.Printf("Serving https://%s/ ...", k.hostname)
		if err := tshttpSrv.Serve(ts443); err != nil {
			log.Fatalf("failed to start https server in Tailscale: %s", err)
		}
	}()

	go func() {
		ts80, err := tsSrv.Listen("tcp", ":80")
		if err != nil {
			log.Printf("failed to start http server: %s", err)
		}

		log.Printf("Serving http://%s/ ...", k.hostname)
		if err := tshttpSrv.Serve(ts80); err != nil {
			log.Fatalf("failed to start http server in Tailscale: %s", err)
		}
	}()

	localListen, err := net.Listen("tcp", k.localAddr)
	if err != nil {
		return err
	}

	log.Printf("Serving http://%s/ ...", k.localAddr)
	if err := httpSrv.Serve(localListen); err != nil {
		return err
	}

	return nil
}

func firstLabel(s string) string {
	s, _, _ = strings.Cut(s, ".")
	return s
}
