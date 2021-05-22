package bot

import (
	"bufio"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

func ping(roomID, msg string) {
	split := strings.Split(msg, " ")
	if len(split) < 2 {
		return
	}
	count := 5
	if len(split) > 2 {
		if i, err := strconv.Atoi(split[2]); err == nil && i > 0 && i <= 20 {
			count = i
		}
	}
	command := "ping"
	countFlag := "-c"
	if runtime.GOOS == "windows" {
		countFlag = "-n"
	}
	cmd := exec.Command(command, countFlag, strconv.Itoa(count), split[1])

	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		client.SendMessage(roomID, err.Error())
		return
	}

	scanner := bufio.NewScanner(cmdReader)
	go func() {
		outChan, done := client.SendStreamingMessage(roomID)
		var output []string
		for scanner.Scan() {
			output = append(output, scanner.Text())
			outChan <- strings.Join(output, "\n")
		}
		close(done)
		if err = cmd.Wait(); err != nil {
			client.SendMessage(roomID, err.Error())
		}
	}()

	err = cmd.Start()
	if err != nil {
		client.SendMessage(roomID, err.Error())
		return
	}
}
