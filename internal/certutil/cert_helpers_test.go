package certutil

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/matt-gp/otel-lgtm-proxy/internal/config"
)

func TestTLSEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *config.TLSConfig
		expected bool
	}{
		{
			name: "all fields provided",
			config: &config.TLSConfig{
				CertFile: "cert.pem",
				KeyFile:  "key.pem",
				CAFile:   "ca.pem",
			},
			expected: true,
		},
		{
			name: "missing cert file",
			config: &config.TLSConfig{
				CertFile: "",
				KeyFile:  "key.pem",
				CAFile:   "ca.pem",
			},
			expected: false,
		},
		{
			name: "missing key file",
			config: &config.TLSConfig{
				CertFile: "cert.pem",
				KeyFile:  "",
				CAFile:   "ca.pem",
			},
			expected: false,
		},
		{
			name: "missing ca file",
			config: &config.TLSConfig{
				CertFile: "cert.pem",
				KeyFile:  "key.pem",
				CAFile:   "",
			},
			expected: false,
		},
		{
			name: "all fields empty",
			config: &config.TLSConfig{
				CertFile: "",
				KeyFile:  "",
				CAFile:   "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TLSEnabled(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStringClientAuthType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected tls.ClientAuthType
	}{
		{
			name:     "RequestClientCert",
			input:    "RequestClientCert",
			expected: tls.RequestClientCert,
		},
		{
			name:     "RequireAnyClientCert",
			input:    "RequireAnyClientCert",
			expected: tls.RequireAnyClientCert,
		},
		{
			name:     "VerifyClientCertIfGiven",
			input:    "VerifyClientCertIfGiven",
			expected: tls.VerifyClientCertIfGiven,
		},
		{
			name:     "RequireAndVerifyClientCert",
			input:    "RequireAndVerifyClientCert",
			expected: tls.RequireAndVerifyClientCert,
		},
		{
			name:     "NoClientCert",
			input:    "NoClientCert",
			expected: tls.NoClientCert,
		},
		{
			name:     "unknown value defaults to NoClientCert",
			input:    "unknown",
			expected: tls.NoClientCert,
		},
		{
			name:     "empty string defaults to NoClientCert",
			input:    "",
			expected: tls.NoClientCert,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringClientAuthType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateTLSConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    *config.Endpoint
		wantErr   bool
		errSubstr string
	}{
		{
			name: "missing cert file",
			config: &config.Endpoint{
				Address: "https://localhost:8443",
				Timeout: 30,
				TLS: config.TLSConfig{
					CertFile:       "nonexistent.crt",
					KeyFile:        "nonexistent.key",
					CAFile:         "nonexistent.ca",
					ClientAuthType: "NoClientCert",
				},
			},
			wantErr:   true,
			errSubstr: "no such file or directory",
		},
		{
			name: "missing key file",
			config: &config.Endpoint{
				Address: "https://localhost:8443",
				Timeout: 30,
				TLS: config.TLSConfig{
					CertFile:       "nonexistent.crt",
					KeyFile:        "nonexistent.key",
					CAFile:         "nonexistent.ca",
					ClientAuthType: "NoClientCert",
				},
			},
			wantErr:   true,
			errSubstr: "no such file or directory",
		},
		{
			name: "missing CA file",
			config: &config.Endpoint{
				Address: "https://localhost:8443",
				Timeout: 30,
				TLS: config.TLSConfig{
					CertFile:       "testdata/cert.pem", // This would need to exist for this test
					KeyFile:        "testdata/key.pem",  // This would need to exist for this test
					CAFile:         "nonexistent.ca",
					ClientAuthType: "NoClientCert",
				},
			},
			wantErr:   true,
			errSubstr: "no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlsConfig, err := CreateTLSConfig(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
				assert.Nil(t, tlsConfig)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tlsConfig)
				assert.NotNil(t, tlsConfig.Certificates)
				assert.NotNil(t, tlsConfig.RootCAs)
			}
		})
	}
}
