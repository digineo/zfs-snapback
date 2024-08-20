package zfs

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	pb "gopkg.in/cheggaaa/pb.v1"
)

// Flags are options for a transfer process.
type Flags struct {
	Recursive   bool
	Force       bool
	Progress    bool
	Raw         bool
	Compression string
}

// Transfer is a set of arguments for transferring a single snapshot.
type Transfer struct {
	Source           *Fs
	Destination      *Fs
	PreviousSnapshot string // can be empty
	CurrentSnapshot  string
	Flags            Flags
}

func withStderr(cmd *exec.Cmd) (*exec.Cmd, *bytes.Buffer) {
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	return cmd, stderr
}

func (t *Transfer) recv() *exec.Cmd {
	// Build argument list
	args := []string{"recv"}
	if t.Flags.Force {
		// -F must be passed before the filesystem argument
		args = append(args, "-F")
	}
	args = append(args, t.Destination.fullname)

	return t.Destination.zfs.exec("/sbin/zfs", args...)
}

// send initializes the ZFS send command.
func (t *Transfer) send() *exec.Cmd {
	return t.Source.zfs.Send(t.Source.fullname, t.PreviousSnapshot, t.CurrentSnapshot, t.Flags.Raw, false)
}

// sendSize retrieves the size of the snapshot diff.
func (t *Transfer) sendSize() (int64, error) {
	cmd := t.Source.zfs.Send(t.Source.fullname, t.PreviousSnapshot, t.CurrentSnapshot, t.Flags.Raw, true)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	return parseTransferSize(out)
}

func (t *Transfer) RunWithRetry() error {
	const maxRetries = 2
	retries := 0
	var err error

	for {
		err = t.run()
		if err == nil || retries > maxRetries || !strings.Contains(err.Error(), "dataset is busy") {
			break
		}

		retries++
		log.Printf("dataset is busy, retrying in %d seconds", retries)
		time.Sleep(time.Second * time.Duration(retries))
	}

	return err
}

// Run performs sync.
func (t *Transfer) run() error { //nolint:funlen
	var err error
	var size int64

	if t.Flags.Progress {
		size, err = t.sendSize()
		if err != nil {
			return err
		}
	}

	recvCommand, recvErrOut := withStderr(t.recv())
	sendCommand, sendErrOut := withStderr(t.send())
	in, _ := recvCommand.StdinPipe()
	out, _ := sendCommand.StdoutPipe()

	log.Printf("Running %s | %s\n", strings.Join(sendCommand.Args, " "), strings.Join(recvCommand.Args, " "))

	errMtx := sync.Mutex{}
	setErr := func(e error, cmd *exec.Cmd, errOut *bytes.Buffer) {
		errMtx.Lock()
		defer errMtx.Unlock()

		if err == nil {
			// It is the first failed process
			err = &CommandError{
				Args:   cmd.Args,
				Cause:  e,
				Stderr: errOut.String(),
			}
		}
	}

	// copy routine
	copyData := func() {
		if t.Flags.Progress {
			bar := pb.New64(size)
			bar.Units = pb.U_BYTES
			bar.ShowSpeed = true
			bar.Start()
			if _, e := io.Copy(in, bar.NewProxyReader(out)); e == nil {
				// Set to 100% percent
				bar.Set64(size)
			}
			bar.Finish()
		} else {
			io.Copy(in, out)
		}
	}

	// runs the recv command
	recvWg := sync.WaitGroup{}
	recvWg.Add(1)
	recv := func() {
		if e := recvCommand.Run(); e != nil {
			setErr(e, recvCommand, recvErrOut)
			out.Close()
		}
		recvWg.Done()
	}

	/*
		The following order is important to avoid race conditions:
		1. Starting the send process
		2. Starting io.Copy() and the recv process
		3. Waiting for any process to terminate
	*/
	e := sendCommand.Start()
	if e != nil {
		out.Close()
	} else {
		go recv()
		copyData()
		e = sendCommand.Wait()
		recvWg.Wait()
	}

	if e != nil {
		setErr(e, sendCommand, sendErrOut)
	}

	return err
}

// DoSync create missing file systems on the destination and transfers missing snapshots.
func DoSync(from, to *Fs, flags Flags) error {
	log.Println("Synchronize", from.fullname, "to", to.fullname)

	// any snapshots to be transferred?
	if len(from.snaps) > 0 {
		transfer := Transfer{
			Source:      from,
			Destination: to,
			Flags:       flags,
		}

		var previous string
		var missing []string

		if len(to.snaps) == 0 {
			missing = from.snaps
		} else {
			common := lastCommonSnapshotIndex(from.snaps, to.snaps)
			if common == -1 {
				return fmt.Errorf("%s and %s don't have a common snapshot", from.fullname, to.fullname)
			}
			previous = from.snaps[common]
			missing = from.snaps[common+1:]
		}

		for _, current := range missing {
			transfer.PreviousSnapshot = previous
			transfer.CurrentSnapshot = current

			if err := transfer.RunWithRetry(); err != nil {
				return err
			}
			previous = current
		}
	}

	// synchronize the children
	if flags.Recursive {
		for _, fromChild := range from.Children() {
			// ensure the filesystem exists
			toChild, err := to.CreateIfMissing(fromChild.name)
			if err != nil {
				return err
			}
			err = DoSync(fromChild, toChild, flags)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
