package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
	"google.golang.org/protobuf/proto"
)

func TestRunWritesDefaultAmneziaFile(t *testing.T) {
	list := &routercommon.GeoIPList{
		Entry: []*routercommon.GeoIP{
			{
				CountryCode: "RU",
				Cidr: []*routercommon.CIDR{
					cidrForTest(t, "192.0.2.0/24"),
					cidrForTest(t, "2001:db8::/32"),
				},
			},
		},
	}
	datPath := writeTempFileForTest(t, marshalGeoIPListForTest(t, list))
	outPath := tempPathForTest(t)
	var stdout, stderr bytes.Buffer

	err := run([]string{"--geoip", "--dat", datPath, "--out", outPath, "--exclude-ipv6"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("run() error = %v; stderr = %s", err, stderr.String())
	}

	var got []amneziaEntry
	readJSONForTest(t, outPath, &got)
	want := []amneziaEntry{{Hostname: "192.0.2.0/24", IP: ""}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default output = %#v, want %#v", got, want)
	}
	if !strings.Contains(stdout.String(), "wrote "+outPath+": 1 CIDR entries from ru") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunListsCategories(t *testing.T) {
	list := &routercommon.GeoIPList{
		Entry: []*routercommon.GeoIP{
			{CountryCode: "Telegram"},
			{CountryCode: "RU"},
		},
	}
	datPath := writeTempFileForTest(t, marshalGeoIPListForTest(t, list))
	var stdout, stderr bytes.Buffer

	err := run([]string{"--geoip", "--dat", datPath, "--list-categories"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("run() error = %v; stderr = %s", err, stderr.String())
	}

	if got, want := stdout.String(), "ru\ntelegram\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunListsGeositeCategories(t *testing.T) {
	list := &routercommon.GeoSiteList{
		Entry: []*routercommon.GeoSite{
			{CountryCode: "Telegram"},
			{CountryCode: "RU"},
		},
	}
	datPath := writeTempFileForTest(t, marshalGeoSiteListForTest(t, list))
	var stdout, stderr bytes.Buffer

	err := run([]string{"--geosite", "--dat", datPath, "--list-categories"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("run() error = %v; stderr = %s", err, stderr.String())
	}

	if got, want := stdout.String(), "ru\ntelegram\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunReturnsFlagErrorsAndUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := run([]string{"--help"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("run(--help) error = %v", err)
	}
	if !strings.Contains(stderr.String(), "--exclude-private-nets") {
		t.Fatalf("stderr missing usage:\n%s", stderr.String())
	}
}

func TestRunRequiresExplicitModeWhenAnyFlagIsProvided(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := run([]string{"--out", "cidr_list.json"}, strings.NewReader("\n\n\n"), &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "--geoip or --geosite") {
		t.Fatalf("run() error = %v, want mode selection error", err)
	}
	if strings.Contains(stdout.String(), "No flags provided") {
		t.Fatalf("interactive prompt was shown for flagged run:\n%s", stdout.String())
	}
}

func TestRunListCategoriesRequiresExplicitMode(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := run([]string{"--list-categories"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "--geoip or --geosite") {
		t.Fatalf("run() error = %v, want mode selection error", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunUsesModeSpecificDefaultOutputPath(t *testing.T) {
	t.Run("geoip", func(t *testing.T) {
		list := &routercommon.GeoIPList{
			Entry: []*routercommon.GeoIP{
				{
					CountryCode: "ru",
					Cidr:        []*routercommon.CIDR{cidrForTest(t, "192.0.2.0/24")},
				},
			},
		}
		datPath := writeTempFileForTest(t, marshalGeoIPListForTest(t, list))
		workDir := t.TempDir()
		oldDir, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(workDir); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Chdir(oldDir); err != nil {
				t.Fatal(err)
			}
		})

		var stdout, stderr bytes.Buffer
		err = run([]string{"--geoip", "--dat", datPath}, strings.NewReader(""), &stdout, &stderr)
		if err != nil {
			t.Fatalf("run() error = %v; stderr = %s", err, stderr.String())
		}
		if _, err := os.Stat("cidr_list.json"); err != nil {
			t.Fatalf("cidr_list.json was not written: %v", err)
		}
		if strings.Contains(stdout.String(), "ip_list.json") {
			t.Fatalf("stdout still mentions old default: %q", stdout.String())
		}
	})

	t.Run("geosite", func(t *testing.T) {
		list := &routercommon.GeoSiteList{
			Entry: []*routercommon.GeoSite{
				{
					CountryCode: "category-ru",
					Domain:      []*routercommon.Domain{{Value: "example.ru"}},
				},
			},
		}
		datPath := writeTempFileForTest(t, marshalGeoSiteListForTest(t, list))
		workDir := t.TempDir()
		oldDir, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(workDir); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Chdir(oldDir); err != nil {
				t.Fatal(err)
			}
		})

		var stdout, stderr bytes.Buffer
		err = run([]string{"--geosite", "--dat", datPath}, strings.NewReader(""), &stdout, &stderr)
		if err != nil {
			t.Fatalf("run() error = %v; stderr = %s", err, stderr.String())
		}
		if _, err := os.Stat("domain_list.json"); err != nil {
			t.Fatalf("domain_list.json was not written: %v", err)
		}
	})
}

func TestRunWritesGeositeAmneziaFile(t *testing.T) {
	list := &routercommon.GeoSiteList{
		Entry: []*routercommon.GeoSite{
			{
				CountryCode: "category-ru",
				Domain: []*routercommon.Domain{
					{Type: routercommon.Domain_RootDomain, Value: "example.ru"},
					{Type: routercommon.Domain_Full, Value: "portal.example.ru"},
				},
			},
		},
	}
	datPath := writeTempFileForTest(t, marshalGeoSiteListForTest(t, list))
	outPath := tempPathForTest(t)
	var stdout, stderr bytes.Buffer

	err := run([]string{"--geosite", "--dat", datPath, "--out", outPath}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("run(--geosite) error = %v; stderr = %s", err, stderr.String())
	}

	var got []amneziaEntry
	readJSONForTest(t, outPath, &got)
	want := []amneziaEntry{
		{Hostname: "cdn1.ozonusercontent.com", IP: ""},
		{Hostname: "example.ru", IP: ""},
		{Hostname: "ir.ozone.ru", IP: ""},
		{Hostname: "portal.example.ru", IP: ""},
		{Hostname: "st.ozone.ru", IP: ""},
		{Hostname: "xapi.ozon.ru", IP: ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("geosite output = %#v, want %#v", got, want)
	}
	if !strings.Contains(stdout.String(), "wrote "+outPath+": 6 domain entries from category-ru") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunRejectsBothModes(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := run([]string{"--geoip", "--geosite"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "exactly one mode") {
		t.Fatalf("run() error = %v, want exactly one mode", err)
	}
}

func TestRunInteractiveGeoIPDefaultsWithLocalDat(t *testing.T) {
	list := &routercommon.GeoIPList{
		Entry: []*routercommon.GeoIP{
			{
				CountryCode: "ru",
				Cidr: []*routercommon.CIDR{
					cidrForTest(t, "192.0.2.0/24"),
					cidrForTest(t, "2001:db8::/32"),
				},
			},
		},
	}
	datPath := writeTempFileForTest(t, marshalGeoIPListForTest(t, list))
	outPath := tempPathForTest(t)
	input := strings.Join([]string{
		"",      // --geoip default
		"2",     // local --dat
		datPath, // --dat path
		"",      // default category ru
		"",      // default format amnezia
		outPath, // --out
		"",      // do not list categories
		"",      // do not exclude private nets
		"1",     // exclude IPv6
		"",
	}, "\n")
	var stdout, stderr bytes.Buffer

	err := run(nil, strings.NewReader(input), &stdout, &stderr)
	if err != nil {
		t.Fatalf("run(interactive) error = %v; stdout = %s; stderr = %s", err, stdout.String(), stderr.String())
	}

	var got []amneziaEntry
	readJSONForTest(t, outPath, &got)
	want := []amneziaEntry{{Hostname: "192.0.2.0/24", IP: ""}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("interactive output = %#v, want %#v", got, want)
	}
	if !strings.Contains(stdout.String(), "No flags provided") {
		t.Fatalf("interactive prompt missing from stdout:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "--geoip") || strings.Contains(stdout.String(), "--dat") {
		t.Fatalf("interactive prompt contains flag names:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "\nSelect [1]:") {
		t.Fatalf("interactive prompt does not put Select on a new line:\n%s", stdout.String())
	}
}

func TestPromptOptionsCustomValues(t *testing.T) {
	input := strings.Join([]string{
		"1",
		"3",
		"https://example.com/custom-geoip.dat",
		"telegram,geoip:ru",
		"2",
		"custom.json",
		"1",
		"1",
		"1",
		"",
	}, "\n")
	var stdout bytes.Buffer

	options, err := promptOptions(strings.NewReader(input), &stdout)
	if err != nil {
		t.Fatal(err)
	}

	if !options.geoip || options.geosite {
		t.Fatalf("mode = geoip:%v geosite:%v", options.geoip, options.geosite)
	}
	if options.url != "https://example.com/custom-geoip.dat" {
		t.Fatalf("url = %q", options.url)
	}
	if got, want := options.categories, (stringList{"telegram", "ru"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("categories = %v, want %v", got, want)
	}
	if options.format != "metadata" || options.out != "custom.json" {
		t.Fatalf("format/out = %q/%q", options.format, options.out)
	}
	if !options.listCategories || !options.excludePrivateNets || !options.excludeIPv6 {
		t.Fatalf("bool options = list:%v private:%v ipv6:%v", options.listCategories, options.excludePrivateNets, options.excludeIPv6)
	}
}

func TestPromptOptionsGeositeAndMissingDatPath(t *testing.T) {
	t.Run("geosite", func(t *testing.T) {
		input := strings.Repeat("\n", 8)
		options, err := promptOptions(strings.NewReader("2"+input), io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		if options.geoip || !options.geosite {
			t.Fatalf("mode = geoip:%v geosite:%v", options.geoip, options.geosite)
		}
		if got, want := options.categories, (stringList{"category-ru"}); !reflect.DeepEqual(got, want) {
			t.Fatalf("categories = %v, want %v", got, want)
		}
	})

	t.Run("missing dat path", func(t *testing.T) {
		input := "1\n2\n\n"
		_, err := promptOptions(strings.NewReader(input), io.Discard)
		if err == nil || !strings.Contains(err.Error(), "local dat file path") {
			t.Fatalf("error = %v, want local dat file path", err)
		}
	})
}

func TestPromptChoiceRepromptsInvalidInput(t *testing.T) {
	var stdout bytes.Buffer
	reader := bufio.NewReader(strings.NewReader("bad\n3\n2\n"))

	got, err := promptChoice(reader, &stdout, "mode", []string{"one", "two"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("choice = %d, want 2", got)
	}
	if count := strings.Count(stdout.String(), "Invalid choice"); count != 2 {
		t.Fatalf("invalid prompt count = %d, want 2\n%s", count, stdout.String())
	}
}

func TestPromptBoolDefaultYesAndReadError(t *testing.T) {
	got, err := promptBool(bufio.NewReader(strings.NewReader("\n")), io.Discard, "confirm", true)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("promptBool default true returned false")
	}

	_, err = readPromptLine(bufio.NewReader(errorReader{}))
	if err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("readPromptLine error = %v, want read failed", err)
	}
}

func TestRunReturnsProcessingErrors(t *testing.T) {
	t.Run("parse error", func(t *testing.T) {
		datPath := writeTempFileForTest(t, []byte("not protobuf"))
		var stdout, stderr bytes.Buffer

		err := run([]string{"--geoip", "--dat", datPath}, strings.NewReader(""), &stdout, &stderr)
		if err == nil || !strings.Contains(err.Error(), "parse geoip.dat") {
			t.Fatalf("run() error = %v, want parse geoip.dat", err)
		}
	})

	t.Run("category error", func(t *testing.T) {
		list := &routercommon.GeoIPList{
			Entry: []*routercommon.GeoIP{{CountryCode: "ru"}},
		}
		datPath := writeTempFileForTest(t, marshalGeoIPListForTest(t, list))
		var stdout, stderr bytes.Buffer

		err := run([]string{"--geoip", "--dat", datPath, "--category", "missing"}, strings.NewReader(""), &stdout, &stderr)
		if err == nil || !strings.Contains(err.Error(), "category") {
			t.Fatalf("run() error = %v, want category error", err)
		}
	})

	t.Run("write error", func(t *testing.T) {
		list := &routercommon.GeoIPList{
			Entry: []*routercommon.GeoIP{
				{
					CountryCode: "ru",
					Cidr:        []*routercommon.CIDR{cidrForTest(t, "192.0.2.0/24")},
				},
			},
		}
		datPath := writeTempFileForTest(t, marshalGeoIPListForTest(t, list))
		var stdout, stderr bytes.Buffer

		err := run([]string{"--geoip", "--dat", datPath, "--format", "xml"}, strings.NewReader(""), &stdout, &stderr)
		if err == nil || !strings.Contains(err.Error(), "unknown format") {
			t.Fatalf("run() error = %v, want unknown format", err)
		}
	})
}

func TestStringListSetNormalizesAndSplitsCategories(t *testing.T) {
	var values stringList

	if err := values.Set(" ru, geoip:Telegram ,, RU-WHITELIST "); err != nil {
		t.Fatal(err)
	}

	want := stringList{"ru", "telegram", "ru-whitelist"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("values = %v, want %v", values, want)
	}
	if got := values.String(); got != "ru,telegram,ru-whitelist" {
		t.Fatalf("String() = %q", got)
	}
}

func TestParseGeoIPAndAvailableCategories(t *testing.T) {
	data := marshalGeoIPListForTest(t, &routercommon.GeoIPList{
		Entry: []*routercommon.GeoIP{
			{CountryCode: "Telegram"},
			{CountryCode: "RU"},
		},
	})

	list, err := parseGeoIP(data)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"ru", "telegram"}
	if got := availableCategories(list); !reflect.DeepEqual(got, want) {
		t.Fatalf("availableCategories() = %v, want %v", got, want)
	}
}

func TestParseGeoSiteAndAvailableCategories(t *testing.T) {
	data := marshalGeoSiteListForTest(t, &routercommon.GeoSiteList{
		Entry: []*routercommon.GeoSite{
			{CountryCode: "Telegram"},
			{CountryCode: "RU"},
		},
	})

	list, err := parseGeoSite(data)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"ru", "telegram"}
	if got := availableGeoSiteCategories(list); !reflect.DeepEqual(got, want) {
		t.Fatalf("availableGeoSiteCategories() = %v, want %v", got, want)
	}
}

func TestParseGeoIPErrors(t *testing.T) {
	if _, err := parseGeoIP([]byte("not protobuf")); err == nil {
		t.Fatal("parseGeoIP() with invalid data returned nil error")
	}

	empty := marshalGeoIPListForTest(t, &routercommon.GeoIPList{})
	if _, err := parseGeoIP(empty); err == nil || !strings.Contains(err.Error(), "no entries") {
		t.Fatalf("parseGeoIP(empty) error = %v, want no entries", err)
	}
}

func TestParseGeoSiteErrors(t *testing.T) {
	if _, err := parseGeoSite([]byte("not protobuf")); err == nil {
		t.Fatal("parseGeoSite() with invalid data returned nil error")
	}

	empty := marshalGeoSiteListForTest(t, &routercommon.GeoSiteList{})
	if _, err := parseGeoSite(empty); err == nil || !strings.Contains(err.Error(), "no entries") {
		t.Fatalf("parseGeoSite(empty) error = %v, want no entries", err)
	}
}

func TestLoadGeoIPFromLocalFile(t *testing.T) {
	path := writeTempFileForTest(t, []byte("geoip-data"))

	data, source, err := loadDAT("https://example.invalid/geoip.dat", path)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "geoip-data" {
		t.Fatalf("data = %q", data)
	}
	if source != path {
		t.Fatalf("source = %q, want %q", source, path)
	}
}

func TestLoadGeoIPFromHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "get-cidr-list-for-amnezia/1.0" {
			t.Fatalf("User-Agent = %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("geoip-data"))
	}))
	t.Cleanup(server.Close)

	data, source, err := loadDAT(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "geoip-data" {
		t.Fatalf("data = %q", data)
	}
	if source != server.URL {
		t.Fatalf("source = %q, want %q", source, server.URL)
	}
}

func TestLoadGeoIPErrors(t *testing.T) {
	if _, _, err := loadDAT("https://example.invalid/geoip.dat", tempPathForTest(t)+"-missing"); err == nil {
		t.Fatal("loadGeoIP() with missing local file returned nil error")
	}

	if _, _, err := loadDAT("://bad-url", ""); err == nil {
		t.Fatal("loadGeoIP() with bad URL returned nil error")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}))
	t.Cleanup(server.Close)

	if _, _, err := loadDAT(server.URL, ""); err == nil || !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("loadGeoIP(status error) = %v, want download failed", err)
	}
}

func TestCollectCIDRsNormalizesDeduplicatesAndSorts(t *testing.T) {
	list := &routercommon.GeoIPList{
		Entry: []*routercommon.GeoIP{
			{
				CountryCode: "RU",
				Cidr: []*routercommon.CIDR{
					cidrForTest(t, "203.0.113.0/24"),
					cidrForTest(t, "192.0.2.0/24"),
				},
			},
			{
				CountryCode: "RU-WHITELIST",
				Cidr: []*routercommon.CIDR{
					cidrForTest(t, "192.0.2.0/24"),
					cidrForTest(t, "2001:db8::/32"),
				},
			},
		},
	}

	cidrs, used, err := collectCIDRs(list, []string{"geoip:ru", "ru-whitelist"}, filterOptions{})
	if err != nil {
		t.Fatal(err)
	}

	wantCIDRs := []string{"192.0.2.0/24", "203.0.113.0/24", "2001:db8::/32"}
	if !reflect.DeepEqual(cidrs, wantCIDRs) {
		t.Fatalf("cidrs = %v, want %v", cidrs, wantCIDRs)
	}
	wantUsed := []string{"ru", "ru-whitelist"}
	if !reflect.DeepEqual(used, wantUsed) {
		t.Fatalf("used = %v, want %v", used, wantUsed)
	}
}

func TestCollectCIDRsExcludeIPv6(t *testing.T) {
	list := &routercommon.GeoIPList{
		Entry: []*routercommon.GeoIP{
			{
				CountryCode: "test",
				Cidr: []*routercommon.CIDR{
					cidrForTest(t, "192.0.2.0/24"),
					cidrForTest(t, "2001:db8::/32"),
				},
			},
		},
	}

	cidrs, used, err := collectCIDRs(list, []string{"test"}, filterOptions{excludeIPv6: true})
	if err != nil {
		t.Fatal(err)
	}

	if len(used) != 1 || used[0] != "test" {
		t.Fatalf("used categories = %v, want [test]", used)
	}
	if len(cidrs) != 1 || cidrs[0] != "192.0.2.0/24" {
		t.Fatalf("cidrs = %v, want [192.0.2.0/24]", cidrs)
	}
}

func TestCollectCIDRsExcludePrivateNets(t *testing.T) {
	list := &routercommon.GeoIPList{
		Entry: []*routercommon.GeoIP{
			{
				CountryCode: "test",
				Cidr: []*routercommon.CIDR{
					cidrForTest(t, "10.0.0.0/8"),
					cidrForTest(t, "203.0.113.0/24"),
					cidrForTest(t, "fc00::/7"),
				},
			},
		},
	}

	cidrs, _, err := collectCIDRs(list, []string{"test"}, filterOptions{excludePrivateNets: true})
	if err != nil {
		t.Fatal(err)
	}

	if len(cidrs) != 1 || cidrs[0] != "203.0.113.0/24" {
		t.Fatalf("cidrs = %v, want [203.0.113.0/24]", cidrs)
	}
}

func TestCollectCIDRsErrors(t *testing.T) {
	list := &routercommon.GeoIPList{
		Entry: []*routercommon.GeoIP{
			{
				CountryCode: "test",
				Cidr: []*routercommon.CIDR{
					{Ip: []byte{192, 0, 2, 0}, Prefix: 33},
				},
			},
		},
	}

	if _, _, err := collectCIDRs(list, []string{"missing"}, filterOptions{}); err == nil || !strings.Contains(err.Error(), "--list-categories") {
		t.Fatalf("missing category error = %v", err)
	}

	if _, _, err := collectCIDRs(list, []string{"test"}, filterOptions{}); err == nil || !strings.Contains(err.Error(), "invalid prefix") {
		t.Fatalf("invalid CIDR error = %v", err)
	}
}

func TestCollectDomainsNormalizesDeduplicatesAndSorts(t *testing.T) {
	list := &routercommon.GeoSiteList{
		Entry: []*routercommon.GeoSite{
			{
				CountryCode: "RU",
				Domain: []*routercommon.Domain{
					{Value: "z.example"},
					{Value: "a.example"},
					{Value: ""},
				},
			},
			{
				CountryCode: "RU-WHITELIST",
				Domain: []*routercommon.Domain{
					{Value: "a.example"},
					{Value: "b.example"},
				},
			},
		},
	}

	domains, used, err := collectDomains(list, []string{"geosite:ru", "ru-whitelist"})
	if err != nil {
		t.Fatal(err)
	}

	wantDomains := []string{"a.example", "b.example", "z.example"}
	if !reflect.DeepEqual(domains, wantDomains) {
		t.Fatalf("domains = %v, want %v", domains, wantDomains)
	}
	wantUsed := []string{"ru", "ru-whitelist"}
	if !reflect.DeepEqual(used, wantUsed) {
		t.Fatalf("used = %v, want %v", used, wantUsed)
	}
}

func TestCollectDomainsErrors(t *testing.T) {
	list := &routercommon.GeoSiteList{
		Entry: []*routercommon.GeoSite{{CountryCode: "ru"}},
	}

	_, _, err := collectDomains(list, []string{"missing"})
	if err == nil || !strings.Contains(err.Error(), "--geosite --list-categories") {
		t.Fatalf("error = %v, want --geosite --list-categories", err)
	}
}

func TestEnsureDomainsAddsRequiredDomainsWithoutDuplicates(t *testing.T) {
	got := ensureDomains([]string{
		"z.example",
		"cdn1.ozonusercontent.com",
		"",
		"z.example",
	}, requiredGeoSiteDomains)

	want := []string{
		"cdn1.ozonusercontent.com",
		"ir.ozone.ru",
		"st.ozone.ru",
		"xapi.ozon.ru",
		"z.example",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("domains = %v, want %v", got, want)
	}
}

func TestShouldExcludeCIDR(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		filters filterOptions
		want    bool
	}{
		{"invalid", "not-cidr", filterOptions{excludeIPv6: true, excludePrivateNets: true}, false},
		{"public ipv4", "203.0.113.0/24", filterOptions{excludeIPv6: true, excludePrivateNets: true}, false},
		{"private ipv4", "10.0.0.0/8", filterOptions{excludePrivateNets: true}, true},
		{"private ipv6", "fc00::/7", filterOptions{excludePrivateNets: true}, true},
		{"ipv6", "2001:db8::/32", filterOptions{excludeIPv6: true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldExcludeCIDR(tt.value, tt.filters); got != tt.want {
				t.Fatalf("shouldExcludeCIDR() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCIDRToStringErrors(t *testing.T) {
	tests := []*routercommon.CIDR{
		{Ip: []byte{}, Prefix: 0},
		{Ip: []byte{192, 0, 2, 0}, Prefix: 33},
		{Ip: net.ParseIP("2001:db8::").To16(), Prefix: 129},
	}

	for _, tt := range tests {
		if _, err := cidrToString(tt); err == nil {
			t.Fatalf("cidrToString(%v) returned nil error", tt)
		}
	}
}

func TestSortCIDRsFallsBackToLexicographicForInvalidValues(t *testing.T) {
	values := []string{"zzz", "aaa"}

	sortCIDRs(values)

	want := []string{"aaa", "zzz"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("values = %v, want %v", values, want)
	}
}

func TestSortCIDRsOrdersSameIPByPrefix(t *testing.T) {
	values := []string{"192.0.2.0/25", "192.0.2.0/24"}

	sortCIDRs(values)

	want := []string{"192.0.2.0/24", "192.0.2.0/25"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("values = %v, want %v", values, want)
	}
}

func TestWriteJSONFormats(t *testing.T) {
	categories := []string{"ru"}
	cidrs := []string{"192.0.2.0/24", "2001:db8::/32"}

	t.Run("amnezia", func(t *testing.T) {
		path := tempPathForTest(t)
		if err := writeJSON(path, "source.dat", categories, cidrs, " amnezia "); err != nil {
			t.Fatal(err)
		}

		var got []amneziaEntry
		readJSONForTest(t, path, &got)
		want := []amneziaEntry{
			{Hostname: "192.0.2.0/24", IP: ""},
			{Hostname: "2001:db8::/32", IP: ""},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("amnezia JSON = %#v, want %#v", got, want)
		}
	})

	t.Run("raw-array", func(t *testing.T) {
		path := tempPathForTest(t)
		if err := writeJSON(path, "source.dat", categories, cidrs, "RAW-ARRAY"); err != nil {
			t.Fatal(err)
		}

		var got []string
		readJSONForTest(t, path, &got)
		if !reflect.DeepEqual(got, cidrs) {
			t.Fatalf("raw-array JSON = %v, want %v", got, cidrs)
		}
	})

	t.Run("metadata", func(t *testing.T) {
		path := tempPathForTest(t)
		if err := writeJSON(path, "source.dat", categories, cidrs, "metadata"); err != nil {
			t.Fatal(err)
		}

		var got outputFile
		readJSONForTest(t, path, &got)
		if got.Name != "Russian IP ranges for Amnezia VPN" {
			t.Fatalf("Name = %q", got.Name)
		}
		if got.Generated == "" {
			t.Fatal("Generated is empty")
		}
		if got.Source != "source.dat" {
			t.Fatalf("Source = %q", got.Source)
		}
		if !reflect.DeepEqual(got.Categories, categories) || !reflect.DeepEqual(got.IPs, cidrs) {
			t.Fatalf("metadata JSON = %#v", got)
		}
	})
}

func TestWriteSiteJSONFormats(t *testing.T) {
	categories := []string{"ru"}
	domains := []string{"example.ru", "portal.example.ru"}

	t.Run("amnezia", func(t *testing.T) {
		path := tempPathForTest(t)
		if err := writeSiteJSON(path, "source.dat", categories, domains, "amnezia"); err != nil {
			t.Fatal(err)
		}

		var got []amneziaEntry
		readJSONForTest(t, path, &got)
		want := []amneziaEntry{
			{Hostname: "example.ru", IP: ""},
			{Hostname: "portal.example.ru", IP: ""},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("amnezia JSON = %#v, want %#v", got, want)
		}
	})

	t.Run("raw-array", func(t *testing.T) {
		path := tempPathForTest(t)
		if err := writeSiteJSON(path, "source.dat", categories, domains, "raw-array"); err != nil {
			t.Fatal(err)
		}

		var got []string
		readJSONForTest(t, path, &got)
		if !reflect.DeepEqual(got, domains) {
			t.Fatalf("raw-array JSON = %v, want %v", got, domains)
		}
	})

	t.Run("metadata", func(t *testing.T) {
		path := tempPathForTest(t)
		if err := writeSiteJSON(path, "source.dat", categories, domains, "metadata"); err != nil {
			t.Fatal(err)
		}

		var got siteOutputFile
		readJSONForTest(t, path, &got)
		if got.Name != "Site list for Amnezia VPN" || got.Source != "source.dat" || got.Generated == "" {
			t.Fatalf("metadata JSON = %#v", got)
		}
		if !reflect.DeepEqual(got.Categories, categories) || !reflect.DeepEqual(got.Domains, domains) {
			t.Fatalf("metadata JSON = %#v", got)
		}
	})
}

func TestWriteSiteJSONErrors(t *testing.T) {
	err := writeSiteJSON(tempPathForTest(t), "source.dat", nil, nil, "xml")
	if err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Fatalf("error = %v, want unknown format", err)
	}

	err = writeSiteJSON(t.TempDir(), "source.dat", nil, nil, "raw-array")
	if err == nil {
		t.Fatal("writeSiteJSON() with directory path returned nil error")
	}
}

func TestWriteJSONRejectsUnknownFormat(t *testing.T) {
	err := writeJSON(tempPathForTest(t), "source.dat", nil, nil, "xml")
	if err == nil || !strings.Contains(err.Error(), "unknown format") {
		t.Fatalf("error = %v, want unknown format", err)
	}
}

func TestWriteJSONReturnsWriteError(t *testing.T) {
	err := writeJSON(t.TempDir(), "source.dat", nil, nil, "raw-array")
	if err == nil {
		t.Fatal("writeJSON() with directory path returned nil error")
	}
}

func TestPrintUsageUsesDoubleDashFlags(t *testing.T) {
	var buf bytes.Buffer

	printUsage(&buf, "routing_list_util.exe")

	got := buf.String()
	if !strings.Contains(got, "--exclude-ipv6") || !strings.Contains(got, "--out string") {
		t.Fatalf("usage output missing double-dash flags:\n%s", got)
	}
	if strings.Contains(got, "  -exclude-ipv6") {
		t.Fatalf("usage output contains single-dash flag:\n%s", got)
	}
}

func cidrForTest(t *testing.T, value string) *routercommon.CIDR {
	t.Helper()

	ip, ipNet, err := net.ParseCIDR(value)
	if err != nil {
		t.Fatal(err)
	}
	ones, _ := ipNet.Mask.Size()
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	return &routercommon.CIDR{
		Ip:     []byte(ip),
		Prefix: uint32(ones),
	}
}

func marshalGeoIPListForTest(t *testing.T, list *routercommon.GeoIPList) []byte {
	t.Helper()

	data, err := proto.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func marshalGeoSiteListForTest(t *testing.T, list *routercommon.GeoSiteList) []byte {
	t.Helper()

	data, err := proto.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeTempFileForTest(t *testing.T, data []byte) string {
	t.Helper()

	path := tempPathForTest(t)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func tempPathForTest(t *testing.T) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func readJSONForTest(t *testing.T, path string, value any) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatalf("unmarshal %s: %v\n%s", path, err, data)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}
