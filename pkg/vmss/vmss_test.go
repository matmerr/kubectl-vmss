package vmss

import (
	"testing"
)

func TestParseProviderID(t *testing.T) {
	tests := []struct {
		name    string
		pid     string
		want    *NodeInfo
		wantErr bool
	}{
		{
			name: "valid providerID",
			pid:  "azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/MC_my-rg_my-aks_eastus/providers/Microsoft.Compute/virtualMachineScaleSets/aks-nodepool1-12345678-vmss/virtualMachines/0",
			want: &NodeInfo{
				Subscription:  "00000000-0000-0000-0000-000000000000",
				ResourceGroup: "MC_my-rg_my-aks_eastus",
				VMSSName:      "aks-nodepool1-12345678-vmss",
				InstanceID:    "0",
			},
		},
		{
			name: "valid providerID with large instance ID",
			pid:  "azure:///subscriptions/aaaa-bbbb/resourceGroups/my-rg/providers/Microsoft.Compute/virtualMachineScaleSets/my-vmss/virtualMachines/42",
			want: &NodeInfo{
				Subscription:  "aaaa-bbbb",
				ResourceGroup: "my-rg",
				VMSSName:      "my-vmss",
				InstanceID:    "42",
			},
		},
		{
			name:    "non-azure prefix",
			pid:     "aws:///us-east-1/i-12345",
			wantErr: true,
		},
		{
			name:    "empty string",
			pid:     "",
			wantErr: true,
		},
		{
			name:    "azure prefix but missing parts",
			pid:     "azure:///subscriptions/sub-id",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseProviderID(tt.pid)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Subscription != tt.want.Subscription {
				t.Errorf("Subscription: got %q, want %q", got.Subscription, tt.want.Subscription)
			}
			if got.ResourceGroup != tt.want.ResourceGroup {
				t.Errorf("ResourceGroup: got %q, want %q", got.ResourceGroup, tt.want.ResourceGroup)
			}
			if got.VMSSName != tt.want.VMSSName {
				t.Errorf("VMSSName: got %q, want %q", got.VMSSName, tt.want.VMSSName)
			}
			if got.InstanceID != tt.want.InstanceID {
				t.Errorf("InstanceID: got %q, want %q", got.InstanceID, tt.want.InstanceID)
			}
		})
	}
}

func TestParseRunCommandOutput(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantStdout string
		wantStderr string
		wantErr    bool
	}{
		{
			name:       "stdout only",
			raw:        `{"value":[{"message":"[stdout]\nhello world\n[stderr]\n"}]}`,
			wantStdout: "hello world",
			wantStderr: "",
		},
		{
			name:       "stdout and stderr",
			raw:        `{"value":[{"message":"[stdout]\noutput here\n[stderr]\nwarning msg"}]}`,
			wantStdout: "output here",
			wantStderr: "warning msg",
		},
		{
			name:       "empty stdout",
			raw:        `{"value":[{"message":"[stdout]\n[stderr]\nonly error"}]}`,
			wantStdout: "",
			wantStderr: "only error",
		},
		{
			name:    "invalid json",
			raw:     `not json`,
			wantErr: true,
		},
		{
			name:    "empty value array",
			raw:     `{"value":[]}`,
			wantErr: true,
		},
		{
			name:       "Enable succeeded prefix",
			raw:        `{"value":[{"message":"Enable succeeded: \n[stdout]\nhello world\n[stderr]\n"}]}`,
			wantStdout: "hello world",
			wantStderr: "",
		},
		{
			name:       "Enable succeeded with stderr",
			raw:        `{"value":[{"message":"Enable succeeded: \n[stdout]\noutput\n[stderr]\nsome warning"}]}`,
			wantStdout: "output",
			wantStderr: "some warning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRunCommandOutput(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Stdout != tt.wantStdout {
				t.Errorf("Stdout: got %q, want %q", got.Stdout, tt.wantStdout)
			}
			if got.Stderr != tt.wantStderr {
				t.Errorf("Stderr: got %q, want %q", got.Stderr, tt.wantStderr)
			}
		})
	}
}
