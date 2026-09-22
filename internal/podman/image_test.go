package podman

import (
	"context"
	"strings"
	"testing"
)

func TestImageExists(t *testing.T) {
	fakePodman(t, "exit 0\n")
	ok, err := ImageExists(context.Background(), "busybox:latest")
	if err != nil || !ok {
		t.Errorf("ok=%v err=%v", ok, err)
	}
}

func TestImageMissing(t *testing.T) {
	fakePodman(t, "exit 1\n")
	ok, err := ImageExists(context.Background(), "busybox:latest")
	if err != nil || ok {
		t.Errorf("ok=%v err=%v, want missing without error", ok, err)
	}
}

func TestImageExistsError(t *testing.T) {
	fakePodman(t, "echo 'storage broken'; exit 125\n")
	if _, err := ImageExists(context.Background(), "busybox:latest"); err == nil {
		t.Error("non-1 failures must surface as errors, not missing")
	}
}

func TestImageExistsEmpty(t *testing.T) {
	if _, err := ImageExists(context.Background(), "  "); err == nil {
		t.Error("empty reference must fail")
	}
}

func TestPullImage(t *testing.T) {
	fakePodman(t, `if [ "$1" != "pull" ] || [ "$2" != "busybox:latest" ]; then exit 9; fi
echo "Copied blob sha256:abc"`)
	out, err := PullImage(context.Background(), "busybox:latest")
	if err != nil || !strings.Contains(out, "sha256:abc") {
		t.Errorf("out=%q err=%v", out, err)
	}
}

func TestPullImageError(t *testing.T) {
	fakePodman(t, "echo 'manifest unknown'; exit 1\n")
	if _, err := PullImage(context.Background(), "busybox:latest"); err == nil {
		t.Error("expected error")
	}
}

func TestIsLocalImage(t *testing.T) {
	if !IsLocalImage("localhost/app:latest") {
		t.Error("localhost/ prefix is locally built")
	}
	for _, img := range []string{"docker.io/library/busybox", "quay.io/a/b:1", "busybox"} {
		if IsLocalImage(img) {
			t.Errorf("%q is not local", img)
		}
	}
}
