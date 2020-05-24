package main

import (
	"bufio"
	"log"
	"os"
	"os/exec"
	"runtime"
	"siikabot/matrix"
	"strconv"
	"strings"

	"github.com/matrix-org/gomatrix"
)

var matrixClient matrix.Client

func handleMemberEvent(event *gomatrix.Event) {
	if event.Content["membership"] == "invite" && *event.StateKey == matrixClient.UserID {
		resp, err := matrixClient.Client.JoinRoom(event.RoomID, "", nil)
		if err != nil {
			log.Fatal(err)
		}
		log.Print("Joined room " + resp.RoomID)
	}
}

func handleTextEvent(event *gomatrix.Event) {
	if event.Content["msgtype"] == "m.text" && event.Sender != matrixClient.UserID {
		if strings.HasPrefix(event.Content["body"].(string), "!ping") {
			split := strings.Split(event.Content["body"].(string), " ")
			if len(split) < 2 {
				matrixClient.SendMessage(event.RoomID, "Usage: !ping <host> <count>")
				return
			}
			count := 5
			if len(split) > 2 {
				if i, err := strconv.Atoi(split[2]); err == nil && i > 0 && i <= 20 {
					count = i
				}
			}
			command := "/bin/ping"
			countFlag := "-c"
			if runtime.GOOS == "windows" {
				command = "ping"
				countFlag = "-n"
			}
			cmd := exec.Command(command, countFlag, strconv.Itoa(count), split[1])

			cmdReader, err := cmd.StdoutPipe()
			if err != nil {
				matrixClient.SendMessage(event.RoomID, err.Error())
				return
			}

			scanner := bufio.NewScanner(cmdReader)
			go func() {
				outChan, done := matrixClient.SendStreamingMessage(event.RoomID)
				var output []string
				for scanner.Scan() {
					output = append(output, scanner.Text())
					outChan <- strings.Join(output, "\n")
				}
				close(done)
				if err = cmd.Wait(); err != nil {
					matrixClient.SendMessage(event.RoomID, err.Error())
				}
			}()

			err = cmd.Start()
			if err != nil {
				matrixClient.SendMessage(event.RoomID, err.Error())
				return
			}
		}
	}
}

func main() {
	homeserverURL := ""
	userID := ""
	accessToken := ""

	for _, e := range os.Environ() {
		split := strings.SplitN(e, "=", 2)
		switch split[0] {
		case "SIIKABOT_HOMESERVER_URL":
			homeserverURL = split[1]
		case "SIIKABOT_USER_ID":
			userID = split[1]
		case "SIIKABOT_ACCESS_TOKEN":
			accessToken = split[1]
		}
	}

	if len(os.Args) > 3 {
		homeserverURL = os.Args[1]
		userID = os.Args[2]
		accessToken = os.Args[3]
	}

	if homeserverURL == "" || userID == "" || accessToken == "" {

		log.Fatal("invalid config")
	}
	matrixClient = matrix.NewClient(homeserverURL, userID, accessToken)
	syncer := matrixClient.Syncer
	syncer.OnEventType("m.room.member", handleMemberEvent)
	syncer.OnEventType("m.room.message", handleTextEvent)

	resp, err := matrixClient.Client.SyncRequest(0, "", "", false, "")
	if err != nil {
		log.Fatal(err)
	}
	for roomID, _ := range resp.Rooms.Invite {
		resp, err := matrixClient.Client.JoinRoom(roomID, "", nil)
		if err != nil {
			log.Fatal(err)
		}
		log.Print("Joined room " + resp.RoomID)
	}
	if err = matrixClient.Client.Sync(); err != nil {
		log.Fatal(err)
	}
}
