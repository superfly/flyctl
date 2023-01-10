package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/miekg/dns"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

type AppChecker struct {
	jsonOutput bool
	checks     map[string]string
	color      *iostreams.ColorScheme
	ctx        context.Context
	app        *api.AppCompact
	workDir    string
	appConfig  *app.Config
	apiClient  *api.Client
}

func NewAppChecker(ctx context.Context, jsonOutput bool, color *iostreams.ColorScheme) *AppChecker {
	appName := app.NameFromContext(ctx)
	if appName == "" {
		if !jsonOutput {
			fmt.Println("No app provided; skipping app specific checks")
		}
		return nil
	}

	ac := &AppChecker{
		jsonOutput: jsonOutput,
		checks:     make(map[string]string),
		color:      color,
		ctx:        ctx,
		apiClient:  client.FromContext(ctx).API(),
		workDir:    state.WorkingDirectory(ctx),
		app:        nil,
		appConfig:  nil,
	}

	appCompact, err := ac.apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		if !jsonOutput {
			terminal.Debugf("API error looking up app with name %s: %v\n", appName, err)
		}
		return nil
	}

	if !appCompact.Deployed && appCompact.PlatformVersion != "machines" {
		ac.lprint(color.Yellow, "%s app has not been deployed yet. Skipping app checks. Deploy using `flyctl deploy`.\n", appName)
		return nil
	}

	ac.app = appCompact
	ac.appConfig = app.ConfigFromContext(ctx)
	return ac
}

func (ac *AppChecker) lprint(color func(string) string, fmtstr string, args ...interface{}) {
	if ac.jsonOutput {
		return
	}

	if color != nil {
		fmt.Print(color(fmt.Sprintf(fmtstr, args...)))
	} else {
		fmt.Printf(fmtstr, args...)
	}
}

func (ac *AppChecker) checkAll() map[string]string {
	ac.lprint(nil, "\nApp specific checks for %s:\n", ac.app.Name)

	ipAddresses := ac.checkIpsAllocated()
	ac.checkDnsRecords(ipAddresses)

	relPath, err := filepath.Rel(ac.workDir, ac.appConfig.Path)
	if err == nil && relPath == app.DefaultConfigFileName {
		ac.lprint(nil, "\nBuild checks for %s:\n", ac.app.Name)
		contextSize := ac.checkDockerContext()
		// only show longer .dockerignore message when context size > 50MB
		ac.checkDockerIgnore(contextSize > 50*1024*1024)
	}

	return ac.checks
}

func (ac *AppChecker) checkIpsAllocated() []api.IPAddress {
	ac.lprint(nil, "Checking that app has ip addresses allocated... ")

	ipAddresses, err := ac.apiClient.GetIPAddresses(ac.ctx, ac.app.Name)
	if err != nil {
		ac.lprint(nil, "API error listing IP addresses for app %s: %v\n", ac.app.Name, err)
		return nil
	}

	if len(ipAddresses) > 0 {
		ac.checks["appHasIps"] = "ok"
		ac.lprint(ac.color.Green, "PASSED\n")
	} else {
		ac.checks["appHasIps"] = "No ips"
		ac.lprint(nil, `Nope
	No ip addresses assigned to this app. If the app is not intended to receive traffic, this is fine.
	Otherwise, it likely means that the services configuration is not correctly setup to receive http, tls, tcp, or udp traffic.
	https://fly.io/docs/reference/configuration/#the-services-sections
`)
	}
	return ipAddresses
}

func (ac *AppChecker) checkDnsRecords(ipAddresses []api.IPAddress) {
	v4s := make(map[string]bool)
	v6s := make(map[string]bool)
	for _, ip := range ipAddresses {
		switch ip.Type {
		case "v4":
		case "shared_v4":
			v4s[ip.Address] = true
		case "v6":
			v6s[ip.Address] = true
		default:
			ac.lprint(nil, "Ip address %s has unexpected type '%s'. Please file a bug with this message at https://github.com/superfly/flyctl/issues/new?assignees=&labels=bug&template=flyctl-bug-report.md&title=", ip.Address, ip.Type)
		}
	}
	if len(v4s) == 0 && len(v6s) == 0 {
		ac.lprint(nil, "No ipv4 or ipv6 ip addresses allocated to app %s", ac.app.Name)
		return
	}

	appHostname := ac.app.Hostname
	appFqdn := dns.Fqdn(appHostname)
	dnsClient := &dns.Client{}
	ns, err := getFirstFlyDevNameserver(dnsClient)
	if err != nil {
		ac.lprint(nil, "%s. Can't proceed to check A or AAAA records.\n", err.Error())
		return
	}
	nsAddr := fmt.Sprintf("%s:53", strings.TrimSuffix(ns, "."))

	if len(v4s) > 0 {
		ac.lprint(nil, "Checking A record for %s... ", appHostname)
		err, jsonErr := checkDnsRecords(dnsClient, nsAddr, ac.app.Name, appFqdn, "A", v4s)
		if err == nil {
			ac.lprint(ac.color.Green, "PASSED\n")
			ac.checks["appARecord"] = "ok"
		} else {
			ac.lprint(nil, "%s\n\n", err.Error())
			if jsonErr != "" {
				ac.checks["appARecord"] = jsonErr
			} else {
				ac.checks["appARecord"] = err.Error()
			}
		}
	}

	if len(v6s) > 0 {
		ac.lprint(nil, "Checking AAAA record for %s... ", appHostname)
		err, jsonErr := checkDnsRecords(dnsClient, nsAddr, ac.app.Name, appFqdn, "AAAA", v6s)
		if err == nil {
			ac.lprint(ac.color.Green, "PASSED\n")
			ac.checks["appAAAARecord"] = "ok"
		} else {
			ac.lprint(nil, "%s\n\n", err.Error())
			if jsonErr != "" {
				ac.checks["appAAAARecord"] = jsonErr
			} else {
				ac.checks["appAAAARecord"] = err.Error()
			}
		}
	}
}

func getFirstFlyDevNameserver(dnsClient *dns.Client) (string, error) {
	const resolver = "9.9.9.9:53"
	msg := &dns.Msg{}
	flydev := "fly.dev"
	msg.SetQuestion(dns.Fqdn(flydev), dns.TypeNS)
	msg.RecursionDesired = true
	// TODO: use ipv6 when system supports it
	r, _, err := dnsClient.Exchange(msg, resolver)
	if err != nil {
		return "", err
	}
	if r.Rcode != dns.RcodeSuccess {
		return "", fmt.Errorf("failed to resolve NS record for %s. Got error code: %s", flydev, dns.RcodeToString[r.Rcode])
	}
	for _, a := range r.Answer {
		if ns, ok := a.(*dns.NS); ok {
			return ns.Ns, nil
		}
	}
	return "", fmt.Errorf("no NS records found for %s", flydev)
}

func checkDnsRecords(dnsClient *dns.Client, nsAddr string, appName string, appFqdn string, qType string, appIps map[string]bool) (error, string) {
	msg := &dns.Msg{}
	msg.SetQuestion(appFqdn, dns.StringToType[qType])
	msg.RecursionDesired = true

	r, _, err := dnsClient.Exchange(msg, nsAddr)
	if err != nil {
		return fmt.Errorf("failed to lookup A record for %s: %w", appFqdn, err), ""
	}
	if r.Rcode != dns.RcodeSuccess {
		return fmt.Errorf("invalid result when looking up A record for %s: %s", appFqdn, dns.RcodeToString[r.Rcode]), ""
	}
	dnsIps := make(map[string]bool)
	for _, a := range r.Answer {
		if qType == "A" {
			if aRec, ok := a.(*dns.A); ok {
				dnsIps[aRec.A.String()] = true
			}
		} else if qType == "AAAA" {
			if aRec, ok := a.(*dns.AAAA); ok {
				dnsIps[aRec.AAAA.String()] = true
			}
		}
	}

	ipsOnAppNotInDns := make([]string, 0)
	for appIp := range appIps {
		if _, present := dnsIps[appIp]; !present {
			ipsOnAppNotInDns = append(ipsOnAppNotInDns, appIp)
		}
	}
	ipsInDnsNotInApp := make([]string, 0)
	for dnsIp := range dnsIps {
		if _, present := appIps[dnsIp]; !present {
			ipsInDnsNotInApp = append(ipsInDnsNotInApp, dnsIp)
		}
	}

	if len(ipsOnAppNotInDns) == 0 && len(ipsInDnsNotInApp) == 0 {
		return nil, ""
	} else if len(ipsOnAppNotInDns) > 0 {
		missingIps := strings.Join(ipsOnAppNotInDns, ", ")
		return fmt.Errorf(`Nope
	These IPs are missing from the %s %s record: %s
	This likely means we had an operational issue when we tried to create the record.
	Post in https://community.fly.io/ or send us an email if you have a support plan, and we'll get this fixed`,
			appFqdn, qType, missingIps), fmt.Sprintf("missing these ips from the %s record: %s", qType, missingIps)
	} else { // len(ipsInDnsNotInApp) > 0
		missingIps := strings.Join(ipsInDnsNotInApp, ", ")
		return fmt.Errorf(`Nope
	These IPs are set in the %s record for %s, but they are not associated with the %s app: %s
	This likely means we had an operational issue when we tried to create the record.
	Post in https://community.fly.io/ or send us an email if you have a support plan, and we'll get this fixed`,
			qType, appFqdn, appName, missingIps), fmt.Sprintf("extra ips on %s record not associated with app: %s", qType, missingIps)
	}
}

func (ac *AppChecker) checkDockerContext() int {
	ac.lprint(nil, "Checking docker context size (this may take little bit)... ")
	checkKey := "appDockerContextSizeBytes"
	var dockerfile string
	var err error
	if dockerfile = ac.appConfig.Dockerfile(); dockerfile != "" {
		dockerfile = filepath.Join(filepath.Dir(ac.appConfig.Path), dockerfile)
	}
	if dockerfile != "" {
		dockerfile, err = filepath.Abs(dockerfile)
		if err != nil || !helpers.FileExists(dockerfile) {
			ac.lprint(nil, "Nope, Dockerfile '%s' not found\n", dockerfile)
			return -1
		}
	} else {
		dockerfile = filepath.Join(ac.workDir, "Dockerfile")
		if !helpers.FileExists(dockerfile) {
			dockerfile = filepath.Join(ac.workDir, "dockerfile")
		}
	}
	if dockerfile == "" {
		ac.lprint(nil, "Nope, Dockerfile not found")
		return -1
	}
	archiveInfo, err := imgsrc.CreateArchive(dockerfile, ac.workDir, ac.appConfig.Ignorefile(), true)
	if err != nil {
		ac.lprint(nil, "Nope, failed to create archive\n\t%s", err.Error())
		return -1
	}

	archiveSize := archiveInfo.SizeInBytes
	ac.lprint(ac.color.Green, "PASSED")
	ac.lprint(nil, " (%s)\n", humanize.Bytes(uint64(archiveSize)))
	ac.checks[checkKey] = strconv.Itoa(archiveSize)
	return archiveSize
}

func (ac *AppChecker) checkDockerIgnore(printDetailedMsg bool) {
	if ac.appConfig.Build != nil && ac.appConfig.Build.Image != "" {
		return
	}
	ac.lprint(nil, "Checking for .dockerignore... ")
	checkKey := "appDockerIgnore"
	fullPath := filepath.Join(ac.workDir, ".dockerignore")
	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
		ac.checks[checkKey] = "no .dockerignore file found"
		ac.lprint(nil, "Nope\n")
		if printDetailedMsg {
			ac.lprint(nil, `			Found no .dockerignore to limit docker context size. Large docker contexts can slow down builds.
			Create a .dockerignore file to indicate which files and directories may be ignored when building the docker image for this app.
			More info at: https://docs.docker.com/engine/reference/builder/#dockerignore-file`)
			ac.lprint(nil, "\n")
		}
		return
	}
	ac.lprint(ac.color.Green, "PASSED\n")
	ac.checks[checkKey] = "ok"
}
