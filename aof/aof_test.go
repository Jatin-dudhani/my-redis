package aof

import (
	"os"
	"testing"

	"github.com/Jatin-dudhani/my-redis/resp"
)

func TestAOFWriteAndReplay(t *testing.T) {
	tmp := t.TempDir() + "/test.aof"
	a, err := Open(tmp, FsyncAlways)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer a.Close()

	cmd1 := resp.Array([]resp.Value{
		resp.BulkString("SET"),
		resp.BulkString("foo"),
		resp.BulkString("bar"),
	})
	cmd2 := resp.Array([]resp.Value{
		resp.BulkString("LPUSH"),
		resp.BulkString("mylist"),
		resp.BulkString("val1"),
	})

	if err := a.Write(cmd1); err != nil {
		t.Fatalf("write cmd1 failed: %v", err)
	}
	if err := a.Write(cmd2); err != nil {
		t.Fatalf("write cmd2 failed: %v", err)
	}

	var replayed []string
	err = a.Replay(func(v resp.Value) error {
		if v.Typ == resp.TypeArray && len(v.Array) > 0 {
			replayed = append(replayed, v.Array[0].Str)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if len(replayed) != 2 || replayed[0] != "SET" || replayed[1] != "LPUSH" {
		t.Fatalf("expected [SET LPUSH], got %v", replayed)
	}
}

func TestAOFRewrite(t *testing.T) {
	tmp := t.TempDir() + "/rewrite.aof"
	a, err := Open(tmp, FsyncAlways)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer a.Close()

	// 100 SET commands for the same key
	for i := 0; i < 100; i++ {
		cmd := resp.Array([]resp.Value{
			resp.BulkString("SET"),
			resp.BulkString("counter"),
			resp.BulkString("val"),
		})
		if err := a.Write(cmd); err != nil {
			t.Fatalf("write failed: %v", err)
		}
	}

	// rewrite into 1 single command
	rewriteCmds := []resp.Value{
		resp.Array([]resp.Value{
			resp.BulkString("SET"),
			resp.BulkString("counter"),
			resp.BulkString("val"),
		}),
	}
	if err := a.Rewrite(rewriteCmds); err != nil {
		t.Fatalf("rewrite failed: %v", err)
	}

	count := 0
	err = a.Replay(func(v resp.Value) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 command after rewrite, got %d", count)
	}

	// Verify we can continue appending after rewrite
	cmdAfter := resp.Array([]resp.Value{
		resp.BulkString("SET"),
		resp.BulkString("newkey"),
		resp.BulkString("newval"),
	})
	if err := a.Write(cmdAfter); err != nil {
		t.Fatalf("write after rewrite failed: %v", err)
	}

	count = 0
	_ = a.Replay(func(v resp.Value) error {
		count++
		return nil
	})
	if count != 2 {
		t.Fatalf("expected 2 commands after append, got %d", count)
	}

	_ = os.Remove(tmp)
}
