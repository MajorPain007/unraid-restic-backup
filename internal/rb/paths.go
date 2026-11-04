package rb

import "strings"

var Mnt = envOr("RB_MNT", "/mnt")

func shareKey(path string) (string, string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 || parts[0] != strings.Trim(Mnt, "/") {
		return "", "", false
	}
	if inList(parts[1], "disks", "remotes", "addons", "rootshare") {
		return "", "", false
	}
	return parts[2], strings.Join(parts[3:], "/"), true
}

func pathWithin(path, dir string) bool {
	ps, pr, okp := shareKey(path)
	ds, dr, okd := shareKey(dir)
	if okp && okd {
		if ps != ds {
			return false
		}
		path = "/" + pr
		dir = "/" + dr
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		path = "/"
	}
	dir = strings.TrimRight(dir, "/")
	if dir == "" {
		dir = "/"
	}
	return dir == "/" || under(path, dir)
}
