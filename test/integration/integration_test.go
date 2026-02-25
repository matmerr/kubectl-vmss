package integration

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/matmerr/kubectl-vmss/pkg/vmss"
)

// mockRunner implements vmss.Runner for integration testing without
// real kubectl / az access.
type mockRunner struct {
	node            string
	container       string
	nodeInfo        *vmss.NodeInfo
	result          *vmss.CommandResult
	capturedScript  string
	resolveNodeErr  error
	resolveVMSSErr  error
	getContainerErr error
	runCommandErr   error
}

func (m *mockRunner) ResolveNodeFromPod(_ context.Context, namespace, pod string) (string, error) {
	if m.resolveNodeErr != nil {
		return "", m.resolveNodeErr
	}
	return m.node, nil
}

func (m *mockRunner) ResolveVMSS(_ context.Context, node string) (*vmss.NodeInfo, error) {
	if m.resolveVMSSErr != nil {
		return nil, m.resolveVMSSErr
	}
	return m.nodeInfo, nil
}

func (m *mockRunner) GetContainerName(_ context.Context, namespace, pod string) (string, error) {
	if m.getContainerErr != nil {
		return "", m.getContainerErr
	}
	return m.container, nil
}

func (m *mockRunner) RunCommand(_ context.Context, info *vmss.NodeInfo, script string) (*vmss.CommandResult, error) {
	m.capturedScript = script
	if m.runCommandErr != nil {
		return nil, m.runCommandErr
	}
	return m.result, nil
}

func defaultMock() *mockRunner {
	return &mockRunner{
		node:      "aks-nodepool1-12345678-vmss000000",
		container: "cilium-agent",
		nodeInfo: &vmss.NodeInfo{
			Subscription:  "00000000-0000-0000-0000-000000000000",
			ResourceGroup: "MC_my-rg_my-aks_eastus",
			VMSSName:      "aks-nodepool1-12345678-vmss",
			InstanceID:    "0",
		},
		result: &vmss.CommandResult{
			Stdout: "fake log output line 1\nfake log output line 2",
			Stderr: "",
		},
	}
}

// --- pods logs tests ---

type logsOptions struct {
	namespace string
	node      string
	tail      int
	previous  bool
	pod       string
	runner    vmss.Runner
}

func (o *logsOptions) Run(ctx context.Context, stdout, stderr *bytes.Buffer) error {
	node := o.node
	container := ""

	if o.pod != "" {
		var err error
		container, err = o.runner.GetContainerName(ctx, o.namespace, o.pod)
		if err != nil {
			return err
		}
		if node == "" {
			node, err = o.runner.ResolveNodeFromPod(ctx, o.namespace, o.pod)
			if err != nil {
				return err
			}
		}
	}

	info, err := o.runner.ResolveVMSS(ctx, node)
	if err != nil {
		return err
	}

	filter := ""
	if container != "" {
		filter = fmt.Sprintf("--name %s", container)
	}
	tailFlag := ""
	if o.tail > 0 {
		tailFlag = fmt.Sprintf(" --tail=%d", o.tail)
	}
	var script string
	if o.previous {
		script = fmt.Sprintf(
			`RUNNING=$(crictl ps %s -q | head -1); ALL=$(crictl ps -a %s -q); if [ -n "$RUNNING" ]; then CID=$(echo "$ALL" | grep -v "$RUNNING" | head -1); else CID=$(echo "$ALL" | head -1); fi; if [ -z "$CID" ]; then echo 'No previous container found for %s' >&2; exit 1; fi; crictl logs%s $CID`,
			filter, filter, container, tailFlag,
		)
	} else {
		script = fmt.Sprintf(
			`CID=$(crictl ps %s -q | head -1); if [ -z "$CID" ]; then CID=$(crictl ps -a %s -q | head -1); fi; if [ -z "$CID" ]; then echo 'No container found for %s' >&2; exit 1; fi; crictl logs%s $CID`,
			filter, filter, container, tailFlag,
		)
	}

	result, err := o.runner.RunCommand(ctx, info, script)
	if err != nil {
		return err
	}
	if result.Stdout != "" {
		fmt.Fprintln(stdout, result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprintln(stderr, result.Stderr)
	}
	return nil
}

func TestIntegration_PodsLogsFromPod(t *testing.T) {
	m := defaultMock()
	o := &logsOptions{
		namespace: "kube-system",
		pod:       "cilium-abc123",
		tail:      0,
		runner:    m,
	}

	var stdout, stderr bytes.Buffer
	if err := o.Run(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout.String() == "" {
		t.Error("expected stdout output")
	}
	if !contains(m.capturedScript, "--name cilium-agent") {
		t.Errorf("script should filter by container name, got: %s", m.capturedScript)
	}
	if contains(m.capturedScript, "--tail=") {
		t.Errorf("script should NOT contain --tail when tail=0, got: %s", m.capturedScript)
	}
}

func TestIntegration_PodsLogsPrevious(t *testing.T) {
	m := defaultMock()
	o := &logsOptions{
		namespace: "kube-system",
		pod:       "cilium-abc123",
		tail:      50,
		previous:  true,
		runner:    m,
	}

	var stdout, stderr bytes.Buffer
	if err := o.Run(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(m.capturedScript, "crictl ps -a") {
		t.Errorf("previous logs should use 'crictl ps -a', got: %s", m.capturedScript)
	}
	if !contains(m.capturedScript, "grep -v") {
		t.Errorf("previous logs should exclude running container, got: %s", m.capturedScript)
	}
	if !contains(m.capturedScript, "--tail=50") {
		t.Errorf("script should respect tail=50, got: %s", m.capturedScript)
	}
}

func TestIntegration_PodsLogsWithNode(t *testing.T) {
	m := defaultMock()
	o := &logsOptions{
		namespace: "kube-system",
		pod:       "cilium-abc123",
		node:      "my-specific-node",
		tail:      0,
		runner:    m,
	}

	var stdout, stderr bytes.Buffer
	if err := o.Run(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout.String() == "" {
		t.Error("expected stdout output")
	}
}

func TestIntegration_ResolveNodeError(t *testing.T) {
	m := defaultMock()
	m.resolveNodeErr = fmt.Errorf("pod not found")
	o := &logsOptions{
		namespace: "kube-system",
		pod:       "nonexistent-pod",
		tail:      0,
		runner:    m,
	}

	var stdout, stderr bytes.Buffer
	err := o.Run(context.Background(), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when pod doesn't exist")
	}
	if !contains(err.Error(), "pod not found") {
		t.Errorf("expected 'pod not found' error, got: %v", err)
	}
}

func TestIntegration_ResolveVMSSError(t *testing.T) {
	m := defaultMock()
	m.resolveVMSSErr = fmt.Errorf("unexpected providerID format")
	o := &logsOptions{
		namespace: "kube-system",
		pod:       "cilium-abc123",
		tail:      0,
		runner:    m,
	}

	var stdout, stderr bytes.Buffer
	err := o.Run(context.Background(), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for bad providerID")
	}
}

func TestIntegration_RunCommandError(t *testing.T) {
	m := defaultMock()
	m.runCommandErr = fmt.Errorf("az vmss run-command failed")
	o := &logsOptions{
		namespace: "kube-system",
		pod:       "cilium-abc123",
		tail:      0,
		runner:    m,
	}

	var stdout, stderr bytes.Buffer
	err := o.Run(context.Background(), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when run-command fails")
	}
}

func TestIntegration_StderrOutput(t *testing.T) {
	m := defaultMock()
	m.result = &vmss.CommandResult{
		Stdout: "some output",
		Stderr: "a warning",
	}
	o := &logsOptions{
		namespace: "kube-system",
		pod:       "cilium-abc123",
		tail:      0,
		runner:    m,
	}

	var stdout, stderr bytes.Buffer
	if err := o.Run(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(stdout.String(), "some output") {
		t.Errorf("expected stdout to contain 'some output', got: %s", stdout.String())
	}
	if !contains(stderr.String(), "a warning") {
		t.Errorf("expected stderr to contain 'a warning', got: %s", stderr.String())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- pods get tests ---

type getPodsOptions struct {
	node   string
	all    bool
	runner vmss.Runner
}

func (o *getPodsOptions) Run(ctx context.Context, stdout, stderr *bytes.Buffer) error {
	if o.node == "" {
		return fmt.Errorf("node name is required")
	}

	info, err := o.runner.ResolveVMSS(ctx, o.node)
	if err != nil {
		return err
	}

	psFlag := ""
	if o.all {
		psFlag = " -a"
	}
	var script string
	if o.all {
		script = "crictl pods -o table && echo '---' && crictl ps" + psFlag + " -o table"
	} else {
		script = "crictl pods -o table && echo '---' && crictl ps -o table"
	}

	result, err := o.runner.RunCommand(ctx, info, script)
	if err != nil {
		return err
	}
	if result.Stdout != "" {
		fmt.Fprintln(stdout, result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprintln(stderr, result.Stderr)
	}
	return nil
}

func TestIntegration_PodsGet(t *testing.T) {
	m := defaultMock()
	m.result = &vmss.CommandResult{
		Stdout: "POD ID\tCREATED\tSTATE\tNAME\nfake-pod-id\t1h\tReady\tcilium-6jnvz",
		Stderr: "",
	}
	o := &getPodsOptions{
		node:   "aks-nodepool1-12345678-vmss000000",
		runner: m,
	}

	var stdout, stderr bytes.Buffer
	if err := o.Run(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(m.capturedScript, "crictl pods") {
		t.Errorf("expected crictl pods in script, got: %s", m.capturedScript)
	}
	if !contains(stdout.String(), "cilium-6jnvz") {
		t.Errorf("expected pod name in output, got: %s", stdout.String())
	}
}

func TestIntegration_PodsGetAll(t *testing.T) {
	m := defaultMock()
	m.result = &vmss.CommandResult{Stdout: "all containers", Stderr: ""}
	o := &getPodsOptions{
		node:   "my-node",
		all:    true,
		runner: m,
	}

	var stdout, stderr bytes.Buffer
	if err := o.Run(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(m.capturedScript, "crictl ps -a") {
		t.Errorf("expected 'crictl ps -a' in script, got: %s", m.capturedScript)
	}
}

func TestIntegration_PodsGetNoNode(t *testing.T) {
	m := defaultMock()
	o := &getPodsOptions{
		runner: m,
	}

	var stdout, stderr bytes.Buffer
	err := o.Run(context.Background(), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when no node specified")
	}
}

// --- get netns tests ---

type getNetnsOptions struct {
	node   string
	runner vmss.Runner
}

func (o *getNetnsOptions) Run(ctx context.Context, stdout, stderr *bytes.Buffer) error {
	if o.node == "" {
		return fmt.Errorf("node name is required")
	}

	info, err := o.runner.ResolveVMSS(ctx, o.node)
	if err != nil {
		return err
	}

	script := `echo "=== Network Namespaces (lsns) ===" && lsns -t net -o NS,PID,USER,COMMAND 2>/dev/null || true && echo "" && echo "=== Named Network Namespaces (ip netns) ===" && ip netns list 2>/dev/null || echo "(none)"`

	result, err := o.runner.RunCommand(ctx, info, script)
	if err != nil {
		return err
	}
	if result.Stdout != "" {
		fmt.Fprintln(stdout, result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprintln(stderr, result.Stderr)
	}
	return nil
}

func TestIntegration_GetNetns(t *testing.T) {
	m := defaultMock()
	m.result = &vmss.CommandResult{
		Stdout: "=== Network Namespaces (lsns) ===\nNS PID USER COMMAND\n4026531992 1 root /sbin/init",
		Stderr: "",
	}
	o := &getNetnsOptions{
		node:   "my-node",
		runner: m,
	}

	var stdout, stderr bytes.Buffer
	if err := o.Run(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(m.capturedScript, "lsns") {
		t.Errorf("expected lsns in script, got: %s", m.capturedScript)
	}
	if !contains(m.capturedScript, "ip netns") {
		t.Errorf("expected 'ip netns' in script, got: %s", m.capturedScript)
	}
}

// --- acn logs tests ---

type acnLogsOptions struct {
	node   string
	tail   int
	runner vmss.Runner
}

func (o *acnLogsOptions) Run(ctx context.Context, stdout, stderr *bytes.Buffer) error {
	if o.node == "" {
		return fmt.Errorf("node name is required")
	}

	info, err := o.runner.ResolveVMSS(ctx, o.node)
	if err != nil {
		return err
	}

	var script string
	if o.tail > 0 {
		script = fmt.Sprintf(`for f in /var/log/azure-vnet.log /var/log/azure-vnet-ipam.log; do
  if [ -f "$f" ]; then
    echo "=== $f (last %d lines) ==="
    tail -n %d "$f"
  fi
done`, o.tail, o.tail)
	} else {
		script = `for f in /var/log/azure-vnet.log /var/log/azure-vnet-ipam.log; do
  if [ -f "$f" ]; then
    echo "=== $f ==="
    cat "$f"
  fi
done`
	}

	result, err := o.runner.RunCommand(ctx, info, script)
	if err != nil {
		return err
	}
	if result.Stdout != "" {
		fmt.Fprintln(stdout, result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprintln(stderr, result.Stderr)
	}
	return nil
}

func TestIntegration_ACNLogs(t *testing.T) {
	m := defaultMock()
	m.result = &vmss.CommandResult{
		Stdout: "=== /var/log/azure-vnet.log ===\nsome log line",
		Stderr: "",
	}
	o := &acnLogsOptions{
		node:   "my-node",
		tail:   100,
		runner: m,
	}

	var stdout, stderr bytes.Buffer
	if err := o.Run(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(m.capturedScript, "azure-vnet.log") {
		t.Errorf("expected azure-vnet.log in script, got: %s", m.capturedScript)
	}
	if !contains(m.capturedScript, "tail -n 100") {
		t.Errorf("expected tail -n 100 in script, got: %s", m.capturedScript)
	}
}

func TestIntegration_ACNLogsNoTail(t *testing.T) {
	m := defaultMock()
	m.result = &vmss.CommandResult{
		Stdout: "=== /var/log/azure-vnet.log ===\nfull log",
		Stderr: "",
	}
	o := &acnLogsOptions{
		node:   "my-node",
		tail:   0,
		runner: m,
	}

	var stdout, stderr bytes.Buffer
	if err := o.Run(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(m.capturedScript, "cat") {
		t.Errorf("expected cat (full file) in script when tail=0, got: %s", m.capturedScript)
	}
	if contains(m.capturedScript, "tail -n") {
		t.Errorf("script should NOT contain tail -n when tail=0, got: %s", m.capturedScript)
	}
}

// --- acn state tests ---

type acnStateOptions struct {
	node   string
	runner vmss.Runner
}

func (o *acnStateOptions) Run(ctx context.Context, stdout, stderr *bytes.Buffer) error {
	if o.node == "" {
		return fmt.Errorf("node name is required")
	}

	info, err := o.runner.ResolveVMSS(ctx, o.node)
	if err != nil {
		return err
	}

	script := `for f in /var/run/azure-vnet.json /var/run/azure-vnet-ipam.json /etc/cni/net.d/10-azure.conflist; do
  if [ -f "$f" ]; then
    echo "=== $f ==="
    cat "$f"
  fi
done`

	result, err := o.runner.RunCommand(ctx, info, script)
	if err != nil {
		return err
	}
	if result.Stdout != "" {
		fmt.Fprintln(stdout, result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprintln(stderr, result.Stderr)
	}
	return nil
}

func TestIntegration_ACNState(t *testing.T) {
	m := defaultMock()
	m.result = &vmss.CommandResult{
		Stdout: `=== /var/run/azure-vnet.json ===\n{"NetworkContainers": {}}`,
		Stderr: "",
	}
	o := &acnStateOptions{
		node:   "my-node",
		runner: m,
	}

	var stdout, stderr bytes.Buffer
	if err := o.Run(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(m.capturedScript, "azure-vnet.json") {
		t.Errorf("expected azure-vnet.json in script, got: %s", m.capturedScript)
	}
	if !contains(m.capturedScript, "10-azure.conflist") {
		t.Errorf("expected 10-azure.conflist in script, got: %s", m.capturedScript)
	}
}

// --- cilium subcommand tests ---

type ciliumTestOptions struct {
	namespace string
	pod       string
	args      []string
	runner    vmss.Runner
}

func (o *ciliumTestOptions) Run(ctx context.Context, stdout, stderr *bytes.Buffer) error {
	node, err := o.runner.ResolveNodeFromPod(ctx, o.namespace, o.pod)
	if err != nil {
		return err
	}

	info, err := o.runner.ResolveVMSS(ctx, node)
	if err != nil {
		return err
	}

	ciliumArgs := ""
	for i, a := range o.args {
		if i > 0 {
			ciliumArgs += " "
		}
		ciliumArgs += a
	}

	// Use the same buildCiliumScript helper as the real command
	script := fmt.Sprintf(`set -e
POD_NAME=%q
CILIUM_ARGS=%q
SANDBOX_ID=$(crictl pods --name "$POD_NAME" -q | head -1)
SANDBOX_PID=$(crictl inspectp "$SANDBOX_ID" 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin)['info']['pid'])" 2>/dev/null || true)
CID=$(crictl ps --name cilium-agent -q | head -1)
if [ -z "$CID" ]; then CID=$(crictl ps -a --name cilium-agent -q | head -1); fi
IMAGE=$(crictl inspect "$CID" 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['info']['config']['image']['image'])" 2>/dev/null || true)
MNT="/tmp/cilium-rootfs-$$"
mkdir -p "$MNT"
ctr -n k8s.io images mount "$IMAGE" "$MNT" >/dev/null 2>&1
CILIUM_BIN="$MNT/usr/bin/cilium-dbg"
nsenter -t "$SANDBOX_PID" -n -- "$CILIUM_BIN" $CILIUM_ARGS
`, o.pod, ciliumArgs)

	result, err := o.runner.RunCommand(ctx, info, script)
	if err != nil {
		return err
	}
	if result.Stdout != "" {
		fmt.Fprintln(stdout, result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprintln(stderr, result.Stderr)
	}
	return nil
}

func TestIntegration_CiliumStatus(t *testing.T) {
	m := defaultMock()
	m.result = &vmss.CommandResult{
		Stdout: "KVStore:     Ok   Disabled\nKubernetes:  Ok   1.28",
		Stderr: "",
	}
	o := &ciliumTestOptions{
		namespace: "kube-system",
		pod:       "cilium-6jnvz",
		args:      []string{"status"},
		runner:    m,
	}

	var stdout, stderr bytes.Buffer
	if err := o.Run(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify the script uses the image-mount + nsenter approach
	if !contains(m.capturedScript, "crictl pods --name") {
		t.Errorf("script should find pod sandbox, got: %s", m.capturedScript)
	}
	if !contains(m.capturedScript, "crictl inspectp") {
		t.Errorf("script should use crictl inspectp for sandbox PID, got: %s", m.capturedScript)
	}
	if !contains(m.capturedScript, "ctr -n k8s.io images mount") {
		t.Errorf("script should mount container image, got: %s", m.capturedScript)
	}
	if !contains(m.capturedScript, "nsenter") {
		t.Errorf("script should use nsenter, got: %s", m.capturedScript)
	}
	if !contains(m.capturedScript, "cilium-dbg") {
		t.Errorf("script should use cilium-dbg binary, got: %s", m.capturedScript)
	}
	if !contains(stdout.String(), "KVStore") {
		t.Errorf("expected cilium output, got: %s", stdout.String())
	}
}

func TestIntegration_CiliumEndpointList(t *testing.T) {
	m := defaultMock()
	m.result = &vmss.CommandResult{
		Stdout: "ENDPOINT   POLICY   IDENTITY   LABELS",
		Stderr: "",
	}
	o := &ciliumTestOptions{
		namespace: "kube-system",
		pod:       "cilium-6jnvz",
		args:      []string{"endpoint", "list"},
		runner:    m,
	}

	var stdout, stderr bytes.Buffer
	if err := o.Run(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(m.capturedScript, "endpoint list") {
		t.Errorf("expected 'endpoint list' in cilium args, got: %s", m.capturedScript)
	}
}

func TestIntegration_CiliumResolveError(t *testing.T) {
	m := defaultMock()
	m.resolveNodeErr = fmt.Errorf("pod not found")
	o := &ciliumTestOptions{
		namespace: "kube-system",
		pod:       "nonexistent",
		args:      []string{"status"},
		runner:    m,
	}

	var stdout, stderr bytes.Buffer
	err := o.Run(context.Background(), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when pod doesn't exist")
	}
	if !contains(err.Error(), "pod not found") {
		t.Errorf("expected 'pod not found' error, got: %v", err)
	}
}
