package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
	"google.golang.org/protobuf/proto"
)

const (
	defaultGeoIPURL   = "https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat"
	defaultGeoSiteURL = "https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geosite.dat"
	defaultGeoIPOut   = "cidr_list.json"
	defaultGeoSiteOut = "domain_list.json"
)

var (
	defaultGeoIPCategories   = []string{"ru"}
	defaultGeoSiteCategories = []string{"category-ru"}
	requiredGeoSiteDomains   = []string{
		"cdn1.ozonusercontent.com",
		"ir.ozone.ru",
		"st.ozone.ru",
		"xapi.ozon.ru",
	}
)

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		part = normalizeCategory(part)
		if part != "" {
			*s = append(*s, part)
		}
	}
	return nil
}

type outputFile struct {
	Name       string   `json:"name"`
	Generated  string   `json:"generated"`
	Source     string   `json:"source"`
	Categories []string `json:"categories"`
	IPs        []string `json:"ips"`
}

type siteOutputFile struct {
	Name       string   `json:"name"`
	Generated  string   `json:"generated"`
	Source     string   `json:"source"`
	Categories []string `json:"categories"`
	Domains    []string `json:"domains"`
}

type amneziaEntry struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
}

type filterOptions struct {
	excludePrivateNets bool
	excludeIPv6        bool
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		exitErr(err)
	}
}

type cliOptions struct {
	geoip              bool
	geosite            bool
	url                string
	out                string
	dat                string
	format             string
	listCategories     bool
	categories         stringList
	excludePrivateNets bool
	excludeIPv6        bool
}

func defaultCLIOptions() cliOptions {
	return cliOptions{
		url:        defaultGeoIPURL,
		out:        "",
		format:     "amnezia",
		categories: defaultStringList(defaultGeoIPCategories),
	}
}

func defaultStringList(values []string) stringList {
	categories := make(stringList, 0, len(values))
	for _, value := range values {
		categories = append(categories, value)
	}
	return categories
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		options, err := promptOptions(stdin, stdout)
		if err != nil {
			return err
		}
		return runWithOptions(options, stdout)
	}

	options := defaultCLIOptions()

	flags := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		printUsage(flags.Output(), flags.Name())
	}
	flags.BoolVar(&options.geoip, "geoip", false, "generate an IP list from geoip.dat")
	flags.BoolVar(&options.geosite, "geosite", false, "generate a site list from geosite.dat")
	flags.StringVar(&options.url, "url", "", "dat file URL")
	flags.StringVar(&options.out, "out", "", "output JSON path")
	flags.StringVar(&options.dat, "dat", "", "read a dat file from disk instead of downloading")
	flags.StringVar(&options.format, "format", "amnezia", "output format: metadata, raw-array, amnezia")
	flags.BoolVar(&options.listCategories, "list-categories", false, "print categories found in the dat file and exit")
	flags.BoolVar(&options.excludePrivateNets, "exclude-private-nets", false, "exclude private IP ranges")
	flags.BoolVar(&options.excludeIPv6, "exclude-ipv6", false, "exclude IPv6 ranges")
	flags.Var(&options.categories, "category", "category to include; repeat or comma-separate values")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	applyModeDefaults(&options, flagWasProvided(flags, "out"))

	return runWithOptions(options, stdout)
}

func flagWasProvided(flags *flag.FlagSet, name string) bool {
	found := false
	flags.Visit(func(flag *flag.Flag) {
		if flag.Name == name {
			found = true
		}
	})
	return found
}

func applyModeDefaults(options *cliOptions, outProvided bool) {
	if options.geosite {
		if len(options.categories) == 0 || sameStringList(options.categories, defaultGeoIPCategories) {
			options.categories = defaultStringList(defaultGeoSiteCategories)
		}
		if options.url == "" {
			options.url = defaultGeoSiteURL
		}
		if !outProvided && options.out == "" {
			options.out = defaultGeoSiteOut
		}
		return
	}
	if options.geoip {
		if len(options.categories) == 0 {
			options.categories = defaultStringList(defaultGeoIPCategories)
		}
		if options.url == "" {
			options.url = defaultGeoIPURL
		}
		if !outProvided && options.out == "" {
			options.out = defaultGeoIPOut
		}
	}
}

func sameStringList(values stringList, defaults []string) bool {
	if len(values) != len(defaults) {
		return false
	}
	for i, value := range values {
		if value != defaults[i] {
			return false
		}
	}
	return true
}

func runWithOptions(options cliOptions, stdout io.Writer) error {
	if options.geoip == options.geosite {
		return errors.New("select exactly one mode: --geoip or --geosite")
	}
	applyModeDefaults(&options, options.out != "")
	if options.geosite {
		return runGeoSite(options, stdout)
	}
	return runGeoIP(options, stdout)
}

func runGeoIP(options cliOptions, stdout io.Writer) error {
	data, source, err := loadDAT(options.url, options.dat)
	if err != nil {
		return err
	}

	list, err := parseGeoIP(data)
	if err != nil {
		return err
	}

	if options.listCategories {
		for _, code := range availableCategories(list) {
			fmt.Fprintln(stdout, code)
		}
		return nil
	}

	cidrs, usedCategories, err := collectCIDRs(list, options.categories, filterOptions{
		excludePrivateNets: options.excludePrivateNets,
		excludeIPv6:        options.excludeIPv6,
	})
	if err != nil {
		return err
	}

	if err := writeJSON(options.out, source, usedCategories, cidrs, options.format); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "wrote %s: %d CIDR entries from %s\n", options.out, len(cidrs), strings.Join(usedCategories, ","))
	return nil
}

func runGeoSite(options cliOptions, stdout io.Writer) error {
	data, source, err := loadDAT(options.url, options.dat)
	if err != nil {
		return err
	}

	list, err := parseGeoSite(data)
	if err != nil {
		return err
	}

	if options.listCategories {
		for _, code := range availableGeoSiteCategories(list) {
			fmt.Fprintln(stdout, code)
		}
		return nil
	}

	domains, usedCategories, err := collectDomains(list, options.categories)
	if err != nil {
		return err
	}
	domains = ensureDomains(domains, requiredGeoSiteDomains)

	if err := writeSiteJSON(options.out, source, usedCategories, domains, options.format); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "wrote %s: %d domain entries from %s\n", options.out, len(domains), strings.Join(usedCategories, ","))
	return nil
}

func promptOptions(stdin io.Reader, stdout io.Writer) (cliOptions, error) {
	options := defaultCLIOptions()
	reader := bufio.NewReader(stdin)

	fmt.Fprintln(stdout, "No flags provided. Choose options by number. Press Enter to use defaults.")

	mode, err := promptChoice(reader, stdout, "Choose what to generate", []string{
		"generate an IP list from geoip.dat",
		"generate a site list from geosite.dat",
	}, 1)
	if err != nil {
		return options, err
	}
	options.geoip = mode == 1
	options.geosite = mode == 2
	applyModeDefaults(&options, false)

	datName := "geoip.dat"
	if options.geosite {
		datName = "geosite.dat"
	}

	source, err := promptChoice(reader, stdout, "Choose where to read the dat file from", []string{
		"download " + datName + " from the default URL",
		"read " + datName + " from a local file",
		"download " + datName + " from a custom URL",
	}, 1)
	if err != nil {
		return options, err
	}
	switch source {
	case 2:
		value, err := promptText(reader, stdout, "local dat file path", "")
		if err != nil {
			return options, err
		}
		if value == "" {
			return options, errors.New("local dat file path is required")
		}
		options.dat = value
	case 3:
		value, err := promptText(reader, stdout, "dat file URL", options.url)
		if err != nil {
			return options, err
		}
		options.url = value
	}

	categories, err := promptText(reader, stdout, "categories to include", options.categories.String())
	if err != nil {
		return options, err
	}
	options.categories = nil
	if err := options.categories.Set(categories); err != nil {
		return options, err
	}

	format, err := promptChoice(reader, stdout, "Choose the output format", []string{
		"amnezia",
		"metadata",
		"raw-array",
	}, 1)
	if err != nil {
		return options, err
	}
	options.format = []string{"amnezia", "metadata", "raw-array"}[format-1]

	out, err := promptText(reader, stdout, "output JSON path", options.out)
	if err != nil {
		return options, err
	}
	options.out = out

	options.listCategories, err = promptBool(reader, stdout, "print categories and exit", false)
	if err != nil {
		return options, err
	}
	if options.geoip {
		options.excludePrivateNets, err = promptBool(reader, stdout, "exclude private IP ranges", false)
		if err != nil {
			return options, err
		}
		options.excludeIPv6, err = promptBool(reader, stdout, "exclude IPv6 ranges", false)
		if err != nil {
			return options, err
		}
	}

	return options, nil
}

func promptChoice(reader *bufio.Reader, stdout io.Writer, label string, choices []string, defaultChoice int) (int, error) {
	for {
		fmt.Fprintf(stdout, "\n%s:\n", label)
		for i, choice := range choices {
			fmt.Fprintf(stdout, "  %d. %s\n", i+1, choice)
		}
		fmt.Fprintf(stdout, "\nSelect [%d]: ", defaultChoice)

		value, err := readPromptLine(reader)
		if err != nil {
			return 0, err
		}
		if value == "" {
			return defaultChoice, nil
		}
		selected, err := strconv.Atoi(value)
		if err == nil && selected >= 1 && selected <= len(choices) {
			return selected, nil
		}
		fmt.Fprintln(stdout, "Invalid choice. Enter one of the listed numbers.")
	}
}

func promptBool(reader *bufio.Reader, stdout io.Writer, label string, defaultValue bool) (bool, error) {
	defaultChoice := 2
	if defaultValue {
		defaultChoice = 1
	}
	selected, err := promptChoice(reader, stdout, label, []string{"yes", "no"}, defaultChoice)
	if err != nil {
		return false, err
	}
	return selected == 1, nil
}

func promptText(reader *bufio.Reader, stdout io.Writer, label, defaultValue string) (string, error) {
	if defaultValue == "" {
		fmt.Fprintf(stdout, "\nEnter the %s: ", label)
	} else {
		fmt.Fprintf(stdout, "\nEnter the %s [%s]: ", label, defaultValue)
	}
	value, err := readPromptLine(reader)
	if err != nil {
		return "", err
	}
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

func readPromptLine(reader *bufio.Reader) (string, error) {
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func printUsage(out io.Writer, name string) {
	fmt.Fprintf(out, "Usage of %s:\n", name)
	fmt.Fprintln(out, "  --geoip")
	fmt.Fprintln(out, "    \tgenerate an IP list from geoip.dat")
	fmt.Fprintln(out, "  --geosite")
	fmt.Fprintln(out, "    \tgenerate a site list from geosite.dat")
	fmt.Fprintln(out, "  --category value")
	fmt.Fprintln(out, "    \tcategory to include; repeat or comma-separate values (default \"ru\" for --geoip, \"category-ru\" for --geosite)")
	fmt.Fprintln(out, "  --dat string")
	fmt.Fprintln(out, "    \tread a dat file from disk instead of downloading")
	fmt.Fprintln(out, "  --exclude-ipv6")
	fmt.Fprintln(out, "    \texclude IPv6 ranges")
	fmt.Fprintln(out, "  --exclude-private-nets")
	fmt.Fprintln(out, "    \texclude private IP ranges")
	fmt.Fprintln(out, "  --format string")
	fmt.Fprintln(out, "    \toutput format: metadata, raw-array, amnezia (default \"amnezia\")")
	fmt.Fprintln(out, "  --list-categories")
	fmt.Fprintln(out, "    \tprint categories found in the dat file and exit")
	fmt.Fprintln(out, "  --out string")
	fmt.Fprintln(out, "    \toutput JSON path (default \"cidr_list.json\" for --geoip, \"domain_list.json\" for --geosite)")
	fmt.Fprintln(out, "  --url string")
	fmt.Fprintf(out, "    \tdat file URL (default depends on --geoip or --geosite)\n")
}

func loadDAT(url, path string) ([]byte, string, error) {
	if path != "" {
		data, err := os.ReadFile(path)
		return data, path, err
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "get-cidr-list-for-amnezia/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", fmt.Errorf("download failed: %s", resp.Status)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), url, nil
}

func parseGeoIP(data []byte) (*routercommon.GeoIPList, error) {
	var list routercommon.GeoIPList
	if err := proto.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse geoip.dat: %w", err)
	}
	if len(list.Entry) == 0 {
		return nil, errors.New("geoip.dat has no entries")
	}
	return &list, nil
}

func parseGeoSite(data []byte) (*routercommon.GeoSiteList, error) {
	var list routercommon.GeoSiteList
	if err := proto.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse geosite.dat: %w", err)
	}
	if len(list.Entry) == 0 {
		return nil, errors.New("geosite.dat has no entries")
	}
	return &list, nil
}

func availableCategories(list *routercommon.GeoIPList) []string {
	categories := make([]string, 0, len(list.Entry))
	for _, entry := range list.Entry {
		categories = append(categories, strings.ToLower(entry.CountryCode))
	}
	sort.Strings(categories)
	return categories
}

func availableGeoSiteCategories(list *routercommon.GeoSiteList) []string {
	categories := make([]string, 0, len(list.Entry))
	for _, entry := range list.Entry {
		categories = append(categories, strings.ToLower(entry.CountryCode))
	}
	sort.Strings(categories)
	return categories
}

func collectCIDRs(list *routercommon.GeoIPList, requested []string, filters filterOptions) ([]string, []string, error) {
	byCategory := make(map[string]*routercommon.GeoIP, len(list.Entry))
	for _, entry := range list.Entry {
		byCategory[strings.ToLower(entry.CountryCode)] = entry
	}

	seen := make(map[string]struct{})
	var cidrs []string
	var used []string

	for _, category := range requested {
		category = normalizeCategory(category)
		entry := byCategory[category]
		if entry == nil {
			return nil, nil, fmt.Errorf("category %q not found; run with --list-categories", category)
		}
		used = append(used, category)

		for _, cidr := range entry.Cidr {
			value, err := cidrToString(cidr)
			if err != nil {
				return nil, nil, fmt.Errorf("category %q: %w", category, err)
			}
			if shouldExcludeCIDR(value, filters) {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			cidrs = append(cidrs, value)
		}
	}

	sortCIDRs(cidrs)
	return cidrs, used, nil
}

func collectDomains(list *routercommon.GeoSiteList, requested []string) ([]string, []string, error) {
	byCategory := make(map[string]*routercommon.GeoSite, len(list.Entry))
	for _, entry := range list.Entry {
		byCategory[strings.ToLower(entry.CountryCode)] = entry
	}

	seen := make(map[string]struct{})
	var domains []string
	var used []string

	for _, category := range requested {
		category = normalizeCategory(category)
		entry := byCategory[category]
		if entry == nil {
			return nil, nil, fmt.Errorf("category %q not found; run with --geosite --list-categories", category)
		}
		used = append(used, category)

		for _, domain := range entry.Domain {
			value := strings.TrimSpace(domain.GetValue())
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			domains = append(domains, value)
		}
	}

	sort.Strings(domains)
	return domains, used, nil
}

func ensureDomains(domains []string, required []string) []string {
	seen := make(map[string]struct{}, len(domains)+len(required))
	result := make([]string, 0, len(domains)+len(required))
	for _, domain := range domains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		result = append(result, domain)
	}
	for _, domain := range required {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		result = append(result, domain)
	}
	sort.Strings(result)
	return result
}

func shouldExcludeCIDR(value string, filters filterOptions) bool {
	ip, _, err := net.ParseCIDR(value)
	if err != nil {
		return false
	}
	if filters.excludeIPv6 && ip.To4() == nil {
		return true
	}
	if filters.excludePrivateNets && ip.IsPrivate() {
		return true
	}
	return false
}

func cidrToString(cidr *routercommon.CIDR) (string, error) {
	ip := net.IP(cidr.Ip)
	if len(cidr.Ip) == net.IPv4len {
		ip = ip.To4()
	}
	if ip == nil || (ip.To4() == nil && ip.To16() == nil) {
		return "", fmt.Errorf("invalid IP bytes %v", cidr.Ip)
	}

	bits := 8 * len(cidr.Ip)
	if ip.To4() != nil {
		bits = 32
	} else if len(cidr.Ip) == net.IPv6len {
		bits = 128
	}

	ones := int(cidr.Prefix)
	if ones < 0 || ones > bits {
		return "", fmt.Errorf("invalid prefix %d for %s", cidr.Prefix, ip.String())
	}
	return (&net.IPNet{IP: ip, Mask: net.CIDRMask(ones, bits)}).String(), nil
}

func normalizeCategory(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "geoip:")
	value = strings.TrimPrefix(value, "geosite:")
	return value
}

func sortCIDRs(cidrs []string) {
	sort.Slice(cidrs, func(i, j int) bool {
		ipI, netI, errI := net.ParseCIDR(cidrs[i])
		ipJ, netJ, errJ := net.ParseCIDR(cidrs[j])
		if errI != nil || errJ != nil {
			return cidrs[i] < cidrs[j]
		}
		if cmp := bytes.Compare(ipI.To16(), ipJ.To16()); cmp != 0 {
			return cmp < 0
		}
		onesI, _ := netI.Mask.Size()
		onesJ, _ := netJ.Mask.Size()
		return onesI < onesJ
	})
}

func writeJSON(path, source string, categories, cidrs []string, format string) error {
	var data any

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "raw-array":
		data = cidrs
	case "amnezia":
		entries := make([]amneziaEntry, 0, len(cidrs))
		for _, cidr := range cidrs {
			entries = append(entries, amneziaEntry{Hostname: cidr, IP: ""})
		}
		data = entries
	case "metadata":
		data = outputFile{
			Name:       "Russian IP ranges for Amnezia VPN",
			Generated:  time.Now().UTC().Format(time.RFC3339),
			Source:     source,
			Categories: categories,
			IPs:        cidrs,
		}
	default:
		return fmt.Errorf("unknown format %q; use metadata, raw-array or amnezia", format)
	}

	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return os.WriteFile(path, encoded, 0644)
}

func writeSiteJSON(path, source string, categories, domains []string, format string) error {
	var data any

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "raw-array":
		data = domains
	case "amnezia":
		entries := make([]amneziaEntry, 0, len(domains))
		for _, domain := range domains {
			entries = append(entries, amneziaEntry{Hostname: domain, IP: ""})
		}
		data = entries
	case "metadata":
		data = siteOutputFile{
			Name:       "Site list for Amnezia VPN",
			Generated:  time.Now().UTC().Format(time.RFC3339),
			Source:     source,
			Categories: categories,
			Domains:    domains,
		}
	default:
		return fmt.Errorf("unknown format %q; use metadata, raw-array or amnezia", format)
	}

	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return os.WriteFile(path, encoded, 0644)
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
