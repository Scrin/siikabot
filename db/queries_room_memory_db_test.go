package db

import (
	"context"
	"testing"
	"time"
)

// How far the bot has thought back over each room, and the memories its passes change

func TestRoomMemoryState(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()

	if state, err := GetRoomMemoryState(ctx, roomA); err != nil || state.HasNewHistory() || state.After != 0 {
		t.Fatalf("a room the bot never talked in has state %+v, %v", state, err)
	}

	storeTurn(t, question(roomA, "$q1", alice, "hi"), "", "hello", "$a1")
	state, err := GetRoomMemoryState(ctx, roomA)
	if err != nil {
		t.Fatal(err)
	}
	if !state.HasNewHistory() || state.After != 0 || state.LatestAt == nil {
		t.Fatalf("a room with a turn and no pass has state %+v", state)
	}
	if due, _ := RoomsWithNewHistory(ctx); len(due) != 1 || due[0].RoomID != roomA || due[0].Until != state.Until {
		t.Errorf("RoomsWithNewHistory() = %+v, want the room up to its latest row", due)
	}

	// A pass that got through everything leaves nothing new
	outcome := "unchanged"
	progress := RoomMemoryProgress{RoomID: roomA, ReflectedUntil: state.Until, ReflectedAt: &chatT0, LastAttemptAt: chatT0, LastOutcome: &outcome}
	if err := SaveRoomMemoryProgress(ctx, progress); err != nil {
		t.Fatal(err)
	}
	if state, _ := GetRoomMemoryState(ctx, roomA); state.HasNewHistory() || state.After != progress.ReflectedUntil ||
		state.LastOutcome == nil || *state.LastOutcome != "unchanged" {
		t.Errorf("after a pass the state is %+v", state)
	}
	if due, _ := RoomsWithNewHistory(ctx); len(due) != 0 {
		t.Errorf("RoomsWithNewHistory() = %+v after a pass, want none", due)
	}

	// A new turn after it is new again, from where the pass left off
	storeTurn(t, question(roomA, "$q2", bob, "more"), "", "sure", "$a2")
	if state, _ := GetRoomMemoryState(ctx, roomA); !state.HasNewHistory() || state.After != progress.ReflectedUntil {
		t.Errorf("after another turn the state is %+v", state)
	}

	// A room with memories but no history is still listed, for the dashboard
	if _, err := ApplyRoomMemoryChanges(ctx, roomB, []RoomMemoryChange{{Action: RoomMemoryAdd, Memory: "likes fish"}},
		RoomMemoryProgress{RoomID: roomB, LastAttemptAt: chatT0}, nil); err != nil {
		t.Fatal(err)
	}
	if states, _ := GetAllRoomMemoryStates(ctx); len(states) != 2 || states[0].RoomID != roomA || states[1].RoomID != roomB {
		t.Errorf("GetAllRoomMemoryStates() = %+v, want both rooms", states)
	}
}

func TestApplyRoomMemoryChanges(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	trace1, trace2 := "trace-1", "trace-2"
	progress := func(until int64) RoomMemoryProgress {
		return RoomMemoryProgress{RoomID: roomA, ReflectedUntil: until, LastAttemptAt: chatT0}
	}

	applied, err := ApplyRoomMemoryChanges(ctx, roomA, []RoomMemoryChange{
		{Action: RoomMemoryAdd, Memory: "talks about EVE"},
		{Action: RoomMemoryAdd, Memory: "prefers short answers"},
	}, progress(5), &trace1)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 2 || applied[0].MemoryID == 0 || applied[1].MemoryID == applied[0].MemoryID {
		t.Fatalf("the adds applied are %+v, want each with its new ID", applied)
	}
	eve, short := applied[0].MemoryID, applied[1].MemoryID
	other, _ := ApplyRoomMemoryChanges(ctx, roomB, []RoomMemoryChange{{Action: RoomMemoryAdd, Memory: "another room's"}},
		RoomMemoryProgress{RoomID: roomB, LastAttemptAt: chatT0}, nil)

	// A memory deleted or belonging to another room can't be changed, and isn't reported as changed
	applied, err = ApplyRoomMemoryChanges(ctx, roomA, []RoomMemoryChange{
		{Action: RoomMemoryUpdate, MemoryID: eve, Memory: "plays EVE Online"},
		{Action: RoomMemoryDelete, MemoryID: short},
		{Action: RoomMemoryUpdate, MemoryID: other[0].MemoryID, Memory: "hijacked"},
		{Action: RoomMemoryDelete, MemoryID: other[0].MemoryID},
		{Action: RoomMemoryUpdate, MemoryID: short, Memory: "already gone"},
	}, progress(9), &trace2)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 2 || applied[0].Action != RoomMemoryUpdate || applied[1].Action != RoomMemoryDelete {
		t.Errorf("the changes applied are %+v, want the update and the delete of the room's own memories", applied)
	}

	memories, _ := GetRoomMemories(ctx, roomA)
	if len(memories) != 1 || memories[0].Memory != "plays EVE Online" || *memories[0].CreatedTraceID != trace1 ||
		*memories[0].UpdatedTraceID != trace2 {
		t.Errorf("the room's memories are %+v", memories)
	}
	if others, _ := GetRoomMemories(ctx, roomB); len(others) != 1 || others[0].Memory != "another room's" {
		t.Errorf("another room's memories are %+v", others)
	}
	if state, _ := GetRoomMemoryState(ctx, roomA); state.After != 9 {
		t.Errorf("the progress is %d, want the second pass's 9", state.After)
	}

	// Changes and progress go in together, or not at all
	if _, err := ApplyRoomMemoryChanges(ctx, roomA, []RoomMemoryChange{
		{Action: RoomMemoryAdd, Memory: "would be added"},
		{Action: RoomMemoryAdd, Memory: ""}, // the table refuses an empty memory
	}, progress(20), nil); err == nil {
		t.Fatal("a change the table refuses gave no error")
	}
	if memories, _ := GetRoomMemories(ctx, roomA); len(memories) != 1 {
		t.Errorf("a failed pass left %d memories, want the 1 before it", len(memories))
	}
	if state, _ := GetRoomMemoryState(ctx, roomA); state.After != 9 {
		t.Errorf("a failed pass moved the progress to %d", state.After)
	}

	// The dashboard deletes them
	if deleted, err := DeleteRoomMemory(ctx, eve); err != nil || deleted == nil || deleted.Memory != "plays EVE Online" {
		t.Errorf("DeleteRoomMemory() = %+v, %v", deleted, err)
	}
	if deleted, _ := DeleteRoomMemory(ctx, eve); deleted != nil {
		t.Error("a memory was deleted twice")
	}
	if deleted, _ := DeleteRoomMemories(ctx, roomB); len(deleted) != 1 {
		t.Errorf("DeleteRoomMemories() deleted %d, want 1", len(deleted))
	}
	if n, _ := CountRoomMemories(ctx); n != 0 {
		t.Errorf("%d memories are left", n)
	}
}

// The timestamps a pass is judged by come from the database
func TestRoomMemoryStateLatestAt(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	storeTurn(t, question(roomA, "$q1", alice, "hi"), "", "hello", "$a1")
	if _, err := pool.Exec(ctx, "UPDATE chat_history SET timestamp = now() - interval '2 hours'"); err != nil {
		t.Fatal(err)
	}
	state, _ := GetRoomMemoryState(ctx, roomA)
	if state.LatestAt == nil || time.Since(*state.LatestAt) < 119*time.Minute {
		t.Errorf("the room's latest row is from %v, want two hours ago", state.LatestAt)
	}
}
