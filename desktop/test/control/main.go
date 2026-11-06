// control bridges Node on Windows to MyGo's official AF_UNIX automation socket.
// Node's Windows net client only supports named pipes; Go supports both.
package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	c, err := net.DialTimeout("unix", os.Args[1], time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(45 * time.Second))
	line, err := bufio.NewReader(os.Stdin).ReadBytes('\n')
	if err != nil {
		return err
	}
	if _, err = c.Write(line); err != nil {
		return err
	}
	line, err = bufio.NewReader(c).ReadBytes('\n')
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(line)
	return err
}
