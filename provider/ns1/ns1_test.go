/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ns1

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	api "gopkg.in/ns1/ns1-go.v2/rest"
	"gopkg.in/ns1/ns1-go.v2/rest/model/dns"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

type MockNS1DomainClient struct {
	mock.Mock
}

func (m *MockNS1DomainClient) CreateRecord(r *dns.Record) (*http.Response, error) {
	return nil, nil
}

func (m *MockNS1DomainClient) DeleteRecord(zone string, domain string, t string) (*http.Response, error) {
	return nil, nil
}

func (m *MockNS1DomainClient) UpdateRecord(r *dns.Record) (*http.Response, error) {
	return nil, nil
}

func (m *MockNS1DomainClient) GetZone(zone string) (*dns.Zone, *http.Response, error) {
	r := &dns.ZoneRecord{
		Domain:   "test.foo.com",
		ShortAns: []string{"2.2.2.2"},
		TTL:      3600,
		Type:     "A",
		ID:       "123456789abcdefghijklmno",
	}
	z := &dns.Zone{
		Zone:    "foo.com",
		Records: []*dns.ZoneRecord{r},
		TTL:     3600,
		ID:      "12345678910111213141516a",
	}

	if zone == "foo.com" {
		return z, nil, nil
	}
	return nil, nil, nil
}

func (m *MockNS1DomainClient) ListZones() ([]*dns.Zone, *http.Response, error) {
	zones := []*dns.Zone{
		{Zone: "foo.com", ID: "12345678910111213141516a"},
		{Zone: "bar.com", ID: "12345678910111213141516b"},
	}
	return zones, nil, nil
}

type MockNS1GetZoneFail struct{}

func (m *MockNS1GetZoneFail) CreateRecord(r *dns.Record) (*http.Response, error) {
	return nil, nil
}

func (m *MockNS1GetZoneFail) DeleteRecord(zone string, domain string, t string) (*http.Response, error) {
	return nil, nil
}

func (m *MockNS1GetZoneFail) UpdateRecord(r *dns.Record) (*http.Response, error) {
	return nil, nil
}

func (m *MockNS1GetZoneFail) GetZone(zone string) (*dns.Zone, *http.Response, error) {
	return nil, nil, api.ErrZoneMissing
}

func (m *MockNS1GetZoneFail) ListZones() ([]*dns.Zone, *http.Response, error) {
	zones := []*dns.Zone{
		{Zone: "foo.com", ID: "12345678910111213141516a"},
		{Zone: "bar.com", ID: "12345678910111213141516b"},
	}
	return zones, nil, nil
}

type MockNS1ListZonesFail struct{}

func (m *MockNS1ListZonesFail) CreateRecord(r *dns.Record) (*http.Response, error) {
	return nil, nil
}

func (m *MockNS1ListZonesFail) DeleteRecord(zone string, domain string, t string) (*http.Response, error) {
	return nil, nil
}

func (m *MockNS1ListZonesFail) UpdateRecord(r *dns.Record) (*http.Response, error) {
	return nil, nil
}

func (m *MockNS1ListZonesFail) GetZone(zone string) (*dns.Zone, *http.Response, error) {
	return &dns.Zone{}, nil, nil
}

func (m *MockNS1ListZonesFail) ListZones() ([]*dns.Zone, *http.Response, error) {
	return nil, nil, fmt.Errorf("no zones available")
}

func TestNS1Records(t *testing.T) {
	provider := &NS1Provider{
		client:         &MockNS1DomainClient{},
		domainFilter:   endpoint.NewDomainFilter([]string{"foo.com."}),
		zoneIDFilter:   provider.NewZoneIDFilter([]string{""}),
		minTTLSeconds:  3600,
		maxRetries:     maxRetries,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
	}
	ctx := context.Background()

	records, err := provider.Records(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, len(records))

	provider.client = &MockNS1GetZoneFail{}
	_, err = provider.Records(ctx)
	require.Error(t, err)

	provider.client = &MockNS1ListZonesFail{}
	_, err = provider.Records(ctx)
	require.Error(t, err)
}

func TestNewNS1Provider(t *testing.T) {
	_ = os.Setenv("NS1_APIKEY", "xxxxxxxxxxxxxxxxx")
	testNS1Config := NS1Config{
		DomainFilter: endpoint.NewDomainFilter([]string{"foo.com."}),
		ZoneIDFilter: provider.NewZoneIDFilter([]string{""}),
		DryRun:       false,
	}
	_, err := NewNS1Provider(testNS1Config)
	require.NoError(t, err)

	_ = os.Unsetenv("NS1_APIKEY")
	_, err = NewNS1Provider(testNS1Config)
	require.Error(t, err)
}

func TestNS1Zones(t *testing.T) {
	provider := &NS1Provider{
		client:         &MockNS1DomainClient{},
		domainFilter:   endpoint.NewDomainFilter([]string{"foo.com."}),
		zoneIDFilter:   provider.NewZoneIDFilter([]string{""}),
		maxRetries:     maxRetries,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
	}

	zones, err := provider.zonesFiltered()
	require.NoError(t, err)

	validateNS1Zones(t, zones, []*dns.Zone{
		{Zone: "foo.com"},
	})
}

func validateNS1Zones(t *testing.T, zones []*dns.Zone, expected []*dns.Zone) {
	require.Len(t, zones, len(expected))

	for i, zone := range zones {
		assert.Equal(t, expected[i].Zone, zone.Zone)
	}
}

func TestNS1BuildRecord(t *testing.T) {
	change := &ns1Change{
		Action: ns1Create,
		Endpoint: &endpoint.Endpoint{
			DNSName:    "new",
			Targets:    endpoint.Targets{"target"},
			RecordType: "A",
		},
	}

	provider := &NS1Provider{
		client:         &MockNS1DomainClient{},
		domainFilter:   endpoint.NewDomainFilter([]string{"foo.com."}),
		zoneIDFilter:   provider.NewZoneIDFilter([]string{""}),
		minTTLSeconds:  300,
		maxRetries:     maxRetries,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
	}

	record := provider.ns1BuildRecord("foo.com", change)
	assert.Equal(t, "foo.com", record.Zone)
	assert.Equal(t, "new.foo.com", record.Domain)
	assert.Equal(t, 300, record.TTL)

	changeWithTTL := &ns1Change{
		Action: ns1Create,
		Endpoint: &endpoint.Endpoint{
			DNSName:    "new-b",
			Targets:    endpoint.Targets{"target"},
			RecordType: "A",
			RecordTTL:  3600,
		},
	}
	record = provider.ns1BuildRecord("foo.com", changeWithTTL)
	assert.Equal(t, "foo.com", record.Zone)
	assert.Equal(t, "new-b.foo.com", record.Domain)
	assert.Equal(t, 3600, record.TTL)
}

func TestNS1ApplyChanges(t *testing.T) {
	changes := &plan.Changes{}
	provider := &NS1Provider{
		client:         &MockNS1DomainClient{},
		maxRetries:     maxRetries,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
	}
	changes.Create = []*endpoint.Endpoint{
		{DNSName: "new.foo.com", Targets: endpoint.Targets{"target"}},
		{DNSName: "new.subdomain.bar.com", Targets: endpoint.Targets{"target"}},
	}
	changes.Delete = []*endpoint.Endpoint{{DNSName: "test.foo.com", Targets: endpoint.Targets{"target"}}}
	changes.UpdateNew = []*endpoint.Endpoint{{DNSName: "test.foo.com", Targets: endpoint.Targets{"target-new"}}}
	err := provider.ApplyChanges(context.Background(), changes)
	require.NoError(t, err)

	// empty changes
	changes.Create = []*endpoint.Endpoint{}
	changes.Delete = []*endpoint.Endpoint{}
	changes.UpdateNew = []*endpoint.Endpoint{}
	err = provider.ApplyChanges(context.Background(), changes)
	require.NoError(t, err)
}

func TestNewNS1Changes(t *testing.T) {
	endpoints := []*endpoint.Endpoint{
		{
			DNSName:    "testa.foo.com",
			Targets:    endpoint.Targets{"target-old"},
			RecordType: "A",
		},
		{
			DNSName:    "testba.bar.com",
			Targets:    endpoint.Targets{"target-new"},
			RecordType: "A",
		},
	}
	expected := []*ns1Change{
		{
			Action:   "ns1Create",
			Endpoint: endpoints[0],
		},
		{
			Action:   "ns1Create",
			Endpoint: endpoints[1],
		},
	}
	changes := newNS1Changes("ns1Create", endpoints)
	require.Len(t, changes, len(expected))
	assert.Equal(t, expected, changes)
}

func TestNewNS1ChangesByZone(t *testing.T) {
	provider := &NS1Provider{
		client:         &MockNS1DomainClient{},
		maxRetries:     maxRetries,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
	}
	zones, _ := provider.zonesFiltered()
	changeSets := []*ns1Change{
		{
			Action: "ns1Create",
			Endpoint: &endpoint.Endpoint{
				DNSName:    "new.foo.com",
				Targets:    endpoint.Targets{"target"},
				RecordType: "A",
			},
		},
		{
			Action: "ns1Create",
			Endpoint: &endpoint.Endpoint{
				DNSName:    "unrelated.bar.com",
				Targets:    endpoint.Targets{"target"},
				RecordType: "A",
			},
		},
		{
			Action: "ns1Delete",
			Endpoint: &endpoint.Endpoint{
				DNSName:    "test.foo.com",
				Targets:    endpoint.Targets{"target"},
				RecordType: "A",
			},
		},
		{
			Action: "ns1Update",
			Endpoint: &endpoint.Endpoint{
				DNSName:    "test.foo.com",
				Targets:    endpoint.Targets{"target-new"},
				RecordType: "A",
			},
		},
	}

	changes := ns1ChangesByZone(zones, changeSets)
	assert.Len(t, changes["bar.com"], 1)
	assert.Len(t, changes["foo.com"], 3)
}

// MockNS1RateLimitAndRetry is a mock client that fails on the first attempt
// and succeeds on the second to test the retry logic.
type MockNS1RateLimitAndRetry struct {
	createAttempts  int
	alwaysRateLimit bool
}

func (m *MockNS1RateLimitAndRetry) CreateRecord(r *dns.Record) (*http.Response, error) {
	m.createAttempts++

	// Cover the expected rate limit case in TestNS1ApplyChangesRateLimitExceeded.
	if m.alwaysRateLimit {
		return &http.Response{StatusCode: http.StatusTooManyRequests}, fmt.Errorf("rate limit exceeded after exceeding retries")
	}

	// Cover the number of attempts to create a record in TestNS1ApplyChangesRateLimitRetry.
	if m.createAttempts == 1 {
		// Fail on the first attempt
		return &http.Response{StatusCode: http.StatusTooManyRequests}, fmt.Errorf("rate limit exceeded")
	}

	// Succeed eentually
	return &http.Response{StatusCode: http.StatusOK}, nil
}

func (m *MockNS1RateLimitAndRetry) DeleteRecord(zone string, domain string, t string) (*http.Response, error) {
	return nil, nil
}

func (m *MockNS1RateLimitAndRetry) UpdateRecord(r *dns.Record) (*http.Response, error) {
	return nil, nil
}

func (m *MockNS1RateLimitAndRetry) GetZone(zone string) (*dns.Zone, *http.Response, error) {
	return &dns.Zone{}, nil, nil
}

func (m *MockNS1RateLimitAndRetry) ListZones() ([]*dns.Zone, *http.Response, error) {
	zones := []*dns.Zone{
		{Zone: "foo.com", ID: "12345678910111213141516a"},
	}
	return zones, nil, nil
}

// TestNS1ApplyChangesRateLimitRetry tests that the provider retries on a rate limit error and eventually succeeds.
func TestNS1ApplyChangesRateLimitRetry(t *testing.T) {
	// Use our stateful mock that fails once, then succeeds.
	mockClient := &MockNS1RateLimitAndRetry{}
	provider := &NS1Provider{
		client: mockClient,
		// Tune retry parameters for the test
		maxRetries:     5,
		initialBackoff: 1 * time.Second,
		maxBackoff:     3 * time.Second,
	}

	// Define a change to be created.
	changes := &plan.Changes{
		Create: []*endpoint.Endpoint{
			{DNSName: "new.foo.com", Targets: endpoint.Targets{"target"}},
		},
	}

	// Apply the changes. The call should succeed because of the retry logic.
	err := provider.ApplyChanges(context.Background(), changes)
	require.NoError(t, err, "ApplyChanges should succeed after a retry")

	// Assert that the mock's CreateRecord method was called exactly twice.
	assert.Equal(t, 2, mockClient.createAttempts, "CreateRecord should be called twice (1 fail + 1 success)")
}

// TestNS1ApplyChangesRateLimitExceeded tests that the provider returns an error after exceeding the maximum number of retries.
func TestNS1ApplyChangesRateLimitExceeded(t *testing.T) {
	// Use our stateful mock that always fails.
	mockClient := &MockNS1RateLimitAndRetry{
		alwaysRateLimit: true,
	}
	provider := &NS1Provider{
		client: mockClient,
		// Tune retry parameters for the test
		maxRetries:     3,
		initialBackoff: 1 * time.Millisecond,
		maxBackoff:     2 * time.Millisecond,
	}

	// Define a change to be created.
	changes := &plan.Changes{
		Create: []*endpoint.Endpoint{
			{DNSName: "new.foo.com", Targets: endpoint.Targets{"target"}},
		},
	}

	// Apply the changes. The call should fail because of the retry logic.
	err := provider.ApplyChanges(context.Background(), changes)
	require.Error(t, err, "ApplyChanges should fail after exceeding max retries")

	// Assert that the mock's CreateRecord method was called exactly maxRetries.
	assert.Equal(t, provider.maxRetries, mockClient.createAttempts, "CreateRecord should be called maxRetries times")
}
