package cmd

import "golang.org/x/sys/unix"

func dupTo(oldfd, newfd int) error { return unix.Dup2(oldfd, newfd) }
