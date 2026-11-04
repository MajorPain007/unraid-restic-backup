package rb

import (
	"os"
	"reflect"
	"testing"
)

func testJob(sources []string) Job {
	return Job{ID: "j00000001", Tags: []string{}, Sources: sources, Excludes: []string{}, IExcludes: []string{},
		Advanced: Advanced{Host: "Tower", ExtraFlags: []string{}}}
}

func testRepo(typ, p string) Repo {
	return Repo{Type: typ, Local: RepoLocal{Path: p}}
}

var testSettings = Settings{DataDir: "/mnt/user/appdata/restic.backup"}

func eqList(t *testing.T, want, got []string) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestOwnExcludesPoolPathOfShareDataDir(t *testing.T) {
	eqList(t, []string{"/mnt/cache/appdata/restic.backup/cache"},
		ownExcludes(testJob([]string{"/boot", "/mnt/user/Daten", "/mnt/cache/appdata"}), testRepo("sftp", ""), testSettings))
}

func TestOwnExcludesSharePathOfPoolDataDir(t *testing.T) {
	eqList(t, []string{"/mnt/user/appdata/restic.backup/cache"},
		ownExcludes(testJob([]string{"/mnt/user/appdata"}), testRepo("sftp", ""),
			Settings{DataDir: "/mnt/cache/appdata/restic.backup"}))
}

func TestOwnExcludesOncePerSpelling(t *testing.T) {
	eqList(t, []string{"/mnt/user/appdata/restic.backup/cache", "/mnt/cache/appdata/restic.backup/cache"},
		ownExcludes(testJob([]string{"/mnt/user/appdata", "/mnt/cache/appdata", "/mnt/user/Daten"}), testRepo("sftp", ""), testSettings))
	eqList(t, []string{},
		ownExcludes(testJob([]string{"/mnt/user/Daten", "/mnt/cache/appdata/plex", "/mnt/disks/usb"}), testRepo("sftp", ""), testSettings))
}

func TestOwnExcludesLocalRepo(t *testing.T) {
	eqList(t, []string{"/mnt/disk3/Backup/restic"},
		ownExcludes(testJob([]string{"/mnt/disk3/Backup"}), testRepo("local", "/mnt/user/Backup/restic"), testSettings))
	eqList(t, []string{},
		ownExcludes(testJob([]string{"/mnt/user/Daten"}), testRepo("local", "/mnt/disks/usb/restic"), testSettings))
	eqList(t, []string{"/mnt/disks/usb/restic"},
		ownExcludes(testJob([]string{"/mnt/disks/usb"}), testRepo("local", "/mnt/disks/usb/restic"), testSettings))
}

func TestJobBackupArgsExcludeFile(t *testing.T) {
	tmp := t.TempDir() + "/op"
	job := testJob([]string{"/mnt/cache/appdata"})
	job.Excludes = []string{"*.tmp"}
	args := backupArgs(job, testRepo("sftp", ""), testSettings, tmp)
	if !inList(tmp+".exclude", args...) {
		t.Errorf("the exclude file is passed: %q", args)
	}
	b, _ := os.ReadFile(tmp + ".exclude")
	if string(b) != "*.tmp\n.zfs/snapshot\n/mnt/cache/appdata/restic.backup/cache\n" {
		t.Errorf("exclude file: %q", b)
	}
}

func TestJobPatternFileDoublesDollar(t *testing.T) {
	if got := patternFile([]string{"$RECYCLE.BIN", "a$$b"}); got != "$$RECYCLE.BIN\na$$$$b\n" {
		t.Errorf("got %q", got)
	}
}

func TestJobForgetArgs(t *testing.T) {
	job := testJob(nil)
	job.Retention = Retention{Daily: 7, Weekly: 4, Within: "1y"}
	eqList(t, []string{"forget", "--json", "--tag", "job:j00000001", "--group-by", "", "--keep-daily", "7",
		"--keep-weekly", "4", "--keep-within", "1y", "--dry-run"}, forgetArgs(job, true))
	eqList(t, []string{"forget", "--json", "--tag", "job:j00000001", "--group-by", "", "--keep-daily", "7",
		"--keep-weekly", "4", "--keep-within", "1y", "--retry-lock", "2m"}, forgetArgs(job, false))
}
