package rb

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAZombieIsNotAlive(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("no /proc")
	}
	cmd := exec.Command("true")
	cmd.Start()
	for i := 0; i < 50; i++ {
		b, _ := os.ReadFile("/proc/" + strconv.Itoa(cmd.Process.Pid) + "/stat")
		if strings.Contains(string(b), ") Z") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pidAlive(cmd.Process.Pid) {
		t.Fatal("an exited child that was not reaped yet counts as alive")
	}
	cmd.Wait()
}
