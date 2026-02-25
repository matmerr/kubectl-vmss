package vmss

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// NodeInfo holds the VMSS coordinates parsed from a node's providerID.
type NodeInfo struct {
	Subscription  string
	ResourceGroup string
	VMSSName      string
	InstanceID    string
}

// CommandResult holds stdout and stderr from a VMSS run-command invocation.
type CommandResult struct {
	Stdout string
	Stderr string
}

// Runner is the interface for executing commands on VMSS instances.
// This abstraction enables testing with mock implementations.
type Runner interface {
	ResolveNodeFromPod(ctx context.Context, namespace, pod string) (string, error)
	ResolveVMSS(ctx context.Context, node string) (*NodeInfo, error)
	GetContainerName(ctx context.Context, namespace, pod string) (string, error)
	RunCommand(ctx context.Context, info *NodeInfo, script string) (*CommandResult, error)
}

// DefaultRunner implements Runner using kubectl and az CLI tools.
type DefaultRunner struct {
	KubectlPath string
	AzPath      string
}

// NewDefaultRunner creates a Runner that shells out to kubectl and az.
func NewDefaultRunner() *DefaultRunner {
	return &DefaultRunner{
		KubectlPath: "kubectl",
		AzPath:      "az",
	}
}

// ResolveNodeFromPod returns the node name a pod is scheduled on.
func (r *DefaultRunner) ResolveNodeFromPod(ctx context.Context, namespace, pod string) (string, error) {
	out, err := r.kubectl(ctx, "get", "pod", pod, "-n", namespace, "-o", "jsonpath={.spec.nodeName}")
	if err != nil {
		return "", fmt.Errorf("could not find pod %s/%s or it has no node assigned: %w", namespace, pod, err)
	}
	node := strings.TrimSpace(out)
	if node == "" {
		return "", fmt.Errorf("pod %s/%s has no node assigned", namespace, pod)
	}
	return node, nil
}

// ResolveVMSS parses the providerID from a node to extract VMSS coordinates.
func (r *DefaultRunner) ResolveVMSS(ctx context.Context, node string) (*NodeInfo, error) {
	fmt.Fprintf(os.Stderr, "Resolving VMSS info from node %s...\n", node)
	out, err := r.kubectl(ctx, "get", "node", node, "-o", "jsonpath={.spec.providerID}")
	if err != nil {
		return nil, fmt.Errorf("could not get providerID for node %s: %w", node, err)
	}
	pid := strings.TrimSpace(out)
	info, err := ParseProviderID(pid)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "  Subscription:  %s\n", info.Subscription)
	fmt.Fprintf(os.Stderr, "  ResourceGroup: %s\n", info.ResourceGroup)
	fmt.Fprintf(os.Stderr, "  VMSS:          %s\n", info.VMSSName)
	fmt.Fprintf(os.Stderr, "  Instance:      %s\n", info.InstanceID)
	return info, nil
}

// ParseProviderID extracts VMSS info from an Azure providerID string.
func ParseProviderID(pid string) (*NodeInfo, error) {
	if !strings.HasPrefix(pid, "azure://") {
		return nil, fmt.Errorf("unexpected providerID format: %s", pid)
	}

	// azure:///subscriptions/<sub>/resourceGroups/<rg>/providers/Microsoft.Compute/virtualMachineScaleSets/<vmss>/virtualMachines/<id>
	parts := strings.Split(pid, "/")
	info := &NodeInfo{}
	for i, p := range parts {
		switch strings.ToLower(p) {
		case "subscriptions":
			if i+1 < len(parts) {
				info.Subscription = parts[i+1]
			}
		case "resourcegroups":
			if i+1 < len(parts) {
				info.ResourceGroup = parts[i+1]
			}
		case "virtualmachinescalesets":
			if i+1 < len(parts) {
				info.VMSSName = parts[i+1]
			}
		case "virtualmachines":
			if i+1 < len(parts) {
				info.InstanceID = parts[i+1]
			}
		}
	}

	if info.Subscription == "" || info.ResourceGroup == "" || info.VMSSName == "" || info.InstanceID == "" {
		return nil, fmt.Errorf("could not parse VMSS info from providerID: %s", pid)
	}
	return info, nil
}

// GetContainerName returns the name of the first container in a pod.
func (r *DefaultRunner) GetContainerName(ctx context.Context, namespace, pod string) (string, error) {
	out, err := r.kubectl(ctx, "get", "pod", pod, "-n", namespace, "-o", "jsonpath={.spec.containers[0].name}")
	if err != nil {
		return "", fmt.Errorf("could not get container name for pod %s/%s: %w", namespace, pod, err)
	}
	return strings.TrimSpace(out), nil
}

// runCommandResponse is the JSON shape returned by az vmss run-command invoke.
type runCommandResponse struct {
	Value []struct {
		Message string `json:"message"`
	} `json:"value"`
}

// RunCommand executes a shell script on a VMSS instance via az vmss run-command invoke.
func (r *DefaultRunner) RunCommand(ctx context.Context, info *NodeInfo, script string) (*CommandResult, error) {
	out, err := r.az(ctx,
		"vmss", "run-command", "invoke",
		"-g", info.ResourceGroup,
		"-n", info.VMSSName,
		"--instance-id", info.InstanceID,
		"--command-id", "RunShellScript",
		"--scripts", script,
		"--subscription", info.Subscription,
		"-o", "json",
	)
	if err != nil {
		return nil, fmt.Errorf("az vmss run-command failed: %w\nOutput: %s", err, out)
	}

	return parseRunCommandOutput(out)
}

func parseRunCommandOutput(raw string) (*CommandResult, error) {
	var resp runCommandResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse run-command JSON output: %w\nRaw: %s", err, raw)
	}
	if len(resp.Value) == 0 {
		return nil, fmt.Errorf("run-command returned no output")
	}

	msg := resp.Value[0].Message
	result := &CommandResult{}

	// Azure run-command output format:
	//   Enable succeeded: \n[stdout]\n<stdout>\n[stderr]\n<stderr>
	// Strip everything up to and including "[stdout]\n".
	if idx := strings.Index(msg, "[stdout]\n"); idx >= 0 {
		msg = msg[idx+len("[stdout]\n"):]
	} else if idx := strings.Index(msg, "[stdout]"); idx >= 0 {
		msg = msg[idx+len("[stdout]"):]
	}

	parts := strings.SplitN(msg, "[stderr]", 2)
	result.Stdout = strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		result.Stderr = strings.TrimSpace(parts[1])
	}
	return result, nil
}

// PickFirstNode returns the name of the first node in the cluster.
func (r *DefaultRunner) PickFirstNode(ctx context.Context) (string, error) {
	out, err := r.kubectl(ctx, "get", "nodes", "-o", "jsonpath={.items[0].metadata.name}")
	if err != nil {
		return "", fmt.Errorf("could not list nodes: %w", err)
	}
	node := strings.TrimSpace(out)
	if node == "" {
		return "", fmt.Errorf("no nodes found in cluster")
	}
	return node, nil
}

func (r *DefaultRunner) kubectl(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.KubectlPath, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (r *DefaultRunner) az(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.AzPath, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
