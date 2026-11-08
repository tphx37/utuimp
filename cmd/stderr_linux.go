package cmd

import "golang.org/x/sys/unix"

func dupTo(oldfd, newfd int) error { return unix.Dup3(oldfd, newfd, 0) }
