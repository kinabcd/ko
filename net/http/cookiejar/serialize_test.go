package cookiejar

import (
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	koT "github.com/kinabcd/ko/testing"
)

// emptyPSL implements PublicSuffixList with just the default
// rule "*".
type emptyPSL struct{}

func (emptyPSL) String() string {
	return "emptyPSL"
}
func (emptyPSL) PublicSuffix(d string) string {
	return d[strings.LastIndex(d, ".")+1:]
}

var serializeTestCookies = []*http.Cookie{{
	Name:       "foo",
	Value:      "bar",
	Path:       "/p",
	Domain:     "example.com",
	Expires:    time.Now(),
	RawExpires: time.Now().Format(time.RFC3339Nano),
	MaxAge:     99,
	Secure:     true,
	HttpOnly:   true,
	Raw:        "raw string",
	Unparsed:   []string{"x", "y", "z"},
}}

var serializeTestURL, _ = url.Parse("http://example.com/x")

func jarEntriesEquals(j1, j2 map[string]map[string]entry) bool {
	return maps.EqualFunc(j1, j2, func(m1, m2 map[string]entry) bool { return maps.EqualFunc(m1, m2, entryEquals) })
}

func entryEquals(e1, e2 entry) bool {
	valuesExcludeTime := func(e entry) []any {
		return []any{e.Name, e.Value, e.Domain, e.Path, e.Secure, e.HttpOnly, e.Persistent, e.HostOnly, e.CanonicalHost}
	}
	valuesTime := func(e entry) []time.Time {
		return []time.Time{e.Expires, e.Creation, e.LastAccess}
	}
	if !slices.Equal(valuesExcludeTime(e1), valuesExcludeTime(e2)) {
		return false
	}
	return slices.EqualFunc(valuesTime(e1), valuesTime(e2), func(t1, t2 time.Time) bool { return t1.Equal(t2) })

}

func TestLoadSave(t *testing.T) {
	d, err1 := os.MkdirTemp("", "")
	koT.AssertNoError(t, err1)
	defer os.RemoveAll(d)
	file := filepath.Join(d, "cookies")
	j := newTestJar()
	j.SetCookies(serializeTestURL, serializeTestCookies)
	koT.AssertNoError(t, j.SaveTo(file))
	_, err2 := os.Stat(file)
	koT.AssertNoError(t, err2)
	j1, _ := Load(file, &Options{})
	koT.Assert(t, len(j1.entries) == len(serializeTestCookies), "%d != %d", len(j1.entries), len(serializeTestCookies))
	koT.Assert(t, jarEntriesEquals(j1.entries, j.entries), "%v and %v not equals", j1.entries, j.entries)
}

func TestMarshalJSON(t *testing.T) {
	j := newTestJar()
	j.SetCookies(serializeTestURL, serializeTestCookies)
	// Marshal the cookies.
	data, err1 := j.MarshalJSON()
	koT.AssertNoError(t, err1)
	// Save them to disk.
	d, err2 := os.MkdirTemp("", "")
	koT.AssertNoError(t, err2)
	defer os.RemoveAll(d)
	file := filepath.Join(d, "cookies")
	koT.AssertNoError(t, os.WriteFile(file, data, 0600))
	// Load cookies from the file.
	j1, _ := Load(file, &Options{})
	koT.Assert(t, len(j1.entries) == len(serializeTestCookies), "%d != %d", len(j1.entries), len(serializeTestCookies))
	koT.Assert(t, jarEntriesEquals(j1.entries, j.entries), "%v and %v not equals", j1.entries, j.entries)
}

func TestLoadNonExistentParent(t *testing.T) {
	d, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatalf("cannot make temp dir: %v", err)
	}
	defer os.RemoveAll(d)
	file := filepath.Join(d, "foo", "cookies")
	_, err = Load(file, &Options{
		PublicSuffixList: testPSL{},
	})
	if err != nil {
		t.Fatalf("cannot make cookie jar: %v", err)
	}
}

func TestLoadNonExistentParentOfParent(t *testing.T) {
	d, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatalf("cannot make temp dir: %v", err)
	}
	defer os.RemoveAll(d)
	file := filepath.Join(d, "foo", "foo", "cookies")
	_, err = Load(file, &Options{
		PublicSuffixList: testPSL{},
	})
	if err != nil {
		t.Fatalf("cannot make cookie jar: %v", err)
	}
}

func TestLoadOldFormat(t *testing.T) {
	// Check that loading the old format (a JSON object)
	// doesn't result in an error.
	f, err := os.CreateTemp("", "cookiejar-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	_, err = f.Write([]byte("{}"))
	koT.AssertNoError(t, err)
	jar, err := Load(f.Name(), &Options{})
	if err != nil {
		t.Errorf("got error: %v", err)
	}
	if jar == nil {
		t.Errorf("nil jar")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	f, err := os.CreateTemp("", "cookiejar-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	_, err = f.Write([]byte("["))
	koT.AssertNoError(t, err)
	jar, err := Load(f.Name(), &Options{})
	if err == nil {
		t.Fatalf("expected error, got none")
	}
	want := "unexpected EOF"
	if ok, _ := regexp.MatchString(want, err.Error()); !ok {
		t.Fatalf("unexpected error message; want %q got %q", want, err.Error())
	}
	if jar != nil {
		t.Fatalf("got nil jar")
	}
}

func TestLoadDifferentPublicSuffixList(t *testing.T) {
	f, err := os.CreateTemp("", "cookiejar-test")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())
	now := tNow
	// With no public suffix list, some domains that should be
	// separate can set cookies for each other.
	jar, err := Load(f.Name(), &Options{
		PublicSuffixList: emptyPSL{},
	})
	if err != nil {
		t.Fatal(err)
	}
	setCookies(jar, "http://foo.co.uk", []string{
		"a=a; max-age=10; domain=.co.uk",
	}, now)
	setCookies(jar, "http://bar.co.uk", []string{
		"b=b; max-age=10; domain=.co.uk",
	}, now)

	// With the default public suffix, the cookies are
	// correctly segmented into their proper domains.
	queries := []query{
		{"http://foo.co.uk/", "a=a b=b"},
		{"http://bar.co.uk/", "a=a b=b"},
	}
	testQueries(t, queries, "no public suffix list", jar, now)
	if err := jar.save(f.Name(), now); err != nil {
		t.Fatalf("cannot save jar: %v", err)
	}

	jar, err = Load(f.Name(), &Options{
		PublicSuffixList: testPSL{},
	})
	if err != nil {
		t.Fatal(err)
	}
	queries = []query{
		{"http://foo.co.uk/", "a=a"},
		{"http://bar.co.uk/", "b=b"},
	}
	testQueries(t, queries, "with test public suffix list", jar, now)
	if err := jar.save(f.Name(), now); err != nil {
		t.Fatalf("cannot save jar: %v", err)
	}

	// When we reload with the original (empty) public suffix
	// we get all the original cookies back.
	jar, err = Load(f.Name(), &Options{
		PublicSuffixList: emptyPSL{},
	})
	if err != nil {
		t.Fatal(err)
	}
	queries = []query{
		{"http://foo.co.uk/", "a=a b=b"},
		{"http://bar.co.uk/", "a=a b=b"},
	}
	testQueries(t, queries, "no public suffix list #2", jar, now)
	if err := jar.save(f.Name(), now); err != nil {
		t.Fatalf("cannot save jar: %v", err)
	}
}

// setCookies sets the given cookies in the given jar associated
// with the given URL at the given time.
func setCookies(jar *Jar, fromURL string, cookies []string, now time.Time) {
	setCookies := make([]*http.Cookie, len(cookies))
	for i, cs := range cookies {
		cookies := (&http.Response{Header: http.Header{"Set-Cookie": {cs}}}).Cookies()
		if len(cookies) != 1 {
			panic(fmt.Sprintf("Wrong cookie line %q: %#v", cs, cookies))
		}
		setCookies[i] = cookies[0]
	}
	jar.setCookies(mustParseURL(fromURL), setCookies, now)
}

func testQueries(t *testing.T, queries []query, description string, jar *Jar, now time.Time) {
	// Test different calls to Cookies.
	for i, query := range queries {
		now = now.Add(1001 * time.Millisecond)
		if got := queryJar(jar, query.toURL, now); got != query.want {
			t.Errorf("Test %q #%d\ngot  %q\nwant %q", description, i, got, query.want)
		}
	}
}

// queryJar returns the results of querying jar for
// cookies associated with url at the given time,
// in "name1=val1 name2=val2" form.
func queryJar(jar *Jar, toURL string, now time.Time) string {
	var s []string
	for _, c := range jar.cookies(mustParseURL(toURL), now) {
		s = append(s, c.Name+"="+c.Value)
	}
	return strings.Join(s, " ")
}

type setCommand struct {
	url     *url.URL
	cookies []*http.Cookie
}

var allCookiesTests = []struct {
	about         string
	set           []setCommand
	expectCookies []*http.Cookie
}{{
	about: "no cookies",
}, {
	about: "a cookie",
	set: []setCommand{{
		url: mustParseURL("https://www.google.com/"),
		cookies: []*http.Cookie{
			{
				Name:    "test-cookie",
				Value:   "test-value",
				Expires: tNow.Add(24 * time.Hour),
			},
		},
	}},
	expectCookies: []*http.Cookie{
		{
			Name:     "test-cookie",
			Value:    "test-value",
			Domain:   "www.google.com",
			Path:     "/",
			Secure:   false,
			HttpOnly: false,
			Expires:  tNow.Add(24 * time.Hour),
		},
	},
}, {
	about: "expired cookie",
	set: []setCommand{{
		url: mustParseURL("https://www.google.com/"),
		cookies: []*http.Cookie{
			{
				Name:    "test-cookie",
				Value:   "test-value",
				Expires: tNow.Add(-24 * time.Hour),
			},
		},
	}},
}, {
	about: "cookie for subpath",
	set: []setCommand{{
		url: mustParseURL("https://www.google.com/subpath/place"),
		cookies: []*http.Cookie{
			{
				Name:    "test-cookie",
				Value:   "test-value",
				Expires: tNow.Add(24 * time.Hour),
			},
		},
	}},
	expectCookies: []*http.Cookie{
		{
			Name:     "test-cookie",
			Value:    "test-value",
			Domain:   "www.google.com",
			Path:     "/subpath",
			Secure:   false,
			HttpOnly: false,
			Expires:  tNow.Add(24 * time.Hour),
		},
	},
}, {
	about: "multiple cookies",
	set: []setCommand{{
		url: mustParseURL("https://www.google.com/"),
		cookies: []*http.Cookie{
			{
				Name:    "test-cookie",
				Value:   "test-value",
				Expires: tNow.Add(24 * time.Hour),
			},
		},
	}, {
		url: mustParseURL("https://www.google.com/subpath/"),
		cookies: []*http.Cookie{
			{
				Name:    "test-cookie",
				Value:   "test-value",
				Expires: tNow.Add(24 * time.Hour),
			},
		},
	}},
	expectCookies: []*http.Cookie{
		{
			Name:     "test-cookie",
			Value:    "test-value",
			Domain:   "www.google.com",
			Path:     "/subpath",
			Secure:   false,
			HttpOnly: false,
			Expires:  tNow.Add(24 * time.Hour),
		},
		{
			Name:     "test-cookie",
			Value:    "test-value",
			Domain:   "www.google.com",
			Path:     "/",
			Secure:   false,
			HttpOnly: false,
			Expires:  tNow.Add(24 * time.Hour),
		},
	},
}}

func cookiesEqual(a, b *http.Cookie) bool {
	return a.Name == b.Name &&
		a.Value == b.Value &&
		a.Domain == b.Domain &&
		a.Path == b.Path &&
		a.Expires.Equal(b.Expires) &&
		a.HttpOnly == b.HttpOnly &&
		a.Secure == b.Secure
}
