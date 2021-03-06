package apiserver

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"

	configv1 "github.com/openshift/api/config/v1"
	configlistersv1 "github.com/openshift/client-go/config/listers/config/v1"

	"github.com/openshift/library-go/pkg/crypto"
	"github.com/openshift/library-go/pkg/operator/events"
)

func TestObserveTLSSecurityProfile(t *testing.T) {
	existingTLSVersion := "VersionTLS11"
	existingCipherSuites := []interface{}{"DES-CBC3-SHA"}

	tests := []struct {
		name                  string
		config                *configv1.TLSSecurityProfile
		expectedMinTLSVersion string
		expectedSuites        []string
	}{
		{
			name:                  "NoAPIServerConfig",
			config:                nil,
			expectedMinTLSVersion: "VersionTLS12",
			expectedSuites:        crypto.OpenSSLToIANACipherSuites(configv1.TLSProfiles[configv1.TLSProfileIntermediateType].Ciphers),
		},
		{
			name: "ModernCrypto",
			config: &configv1.TLSSecurityProfile{
				Type:   configv1.TLSProfileModernType,
				Modern: &configv1.ModernTLSProfile{},
			},
			expectedMinTLSVersion: "VersionTLS13",
			expectedSuites:        crypto.OpenSSLToIANACipherSuites(configv1.TLSProfiles[configv1.TLSProfileModernType].Ciphers),
		},
		{
			name: "OldCrypto",
			config: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileOldType,
				Old:  &configv1.OldTLSProfile{},
			},
			expectedMinTLSVersion: "VersionTLS10",
			expectedSuites:        crypto.OpenSSLToIANACipherSuites(configv1.TLSProfiles[configv1.TLSProfileOldType].Ciphers),
		},
	}
	for _, tt := range tests {
		for _, useAPIServerArgs := range []bool{false, true} {
			var minTLSVersionPath, cipherSuitesPath []string
			if useAPIServerArgs {
				minTLSVersionPath = []string{"apiServerArguments", "tls-min-version"}
				cipherSuitesPath = []string{"apiServerArguments", "tls-cipher-suites"}
			} else {
				minTLSVersionPath = []string{"servingInfo", "minTLSVersion"}
				cipherSuitesPath = []string{"servingInfo", "cipherSuites"}
			}
			t.Run(tt.name, func(t *testing.T) {
				indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
				if tt.config != nil {
					if err := indexer.Add(&configv1.APIServer{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster",
						},
						Spec: configv1.APIServerSpec{
							TLSSecurityProfile: tt.config,
						},
					}); err != nil {
						t.Fatal(err)
					}
				}
				listers := testLister{
					apiLister: configlistersv1.NewAPIServerLister(indexer),
				}

				existingConfig := map[string]interface{}{}
				if err := unstructured.SetNestedField(existingConfig, existingTLSVersion, minTLSVersionPath...); err != nil {
					t.Fatalf("couldn't set existing min TLS version: %v", err)
				}
				if err := unstructured.SetNestedField(existingConfig, existingCipherSuites, cipherSuitesPath...); err != nil {
					t.Fatalf("couldn't set existing cipher suites: %v", err)
				}

				var result map[string]interface{}
				var errs []error
				if useAPIServerArgs {
					result, errs = ObserveTLSSecurityProfileToArguments(listers, events.NewInMemoryRecorder(t.Name()), existingConfig)
				} else {
					result, errs = ObserveTLSSecurityProfile(listers, events.NewInMemoryRecorder(t.Name()), existingConfig)
				}
				if len(errs) > 0 {
					t.Errorf("expected 0 errors, got %v", errs)
				}

				gotMinTLSVersion, _, err := unstructured.NestedString(result, minTLSVersionPath...)
				if err != nil {
					t.Errorf("couldn't get minTLSVersion from the returned object: %v", err)
				}

				gotSuites, _, err := unstructured.NestedStringSlice(result, cipherSuitesPath...)
				if err != nil {
					t.Errorf("couldn't get cipherSuites from the returned object: %v", err)
				}

				if !reflect.DeepEqual(gotSuites, tt.expectedSuites) {
					t.Errorf("got cipherSuites = %v, expected %v", gotSuites, tt.expectedSuites)
				}
				if gotMinTLSVersion != tt.expectedMinTLSVersion {
					t.Errorf("got minTlSVersion = %v, expected %v", gotMinTLSVersion, tt.expectedMinTLSVersion)
				}
			})
		}
	}
}
