package main // or the name of your package

import (
	// "reflect"
	"testing"
	"sort"
)

func TestAssignSecretSantaWithLessThanTwoParticipants(t *testing.T) {
    participants := []Participant{
        {ID: 1, Name: "Alice", Email: "alice@example.com"},
    }

    _, err := AssignSecretSanta(participants)
    if err == nil {
        t.Errorf("AssignSecretSanta() should error out for less than 2 participants")
    } else if err.Error() != "a minimum of 2 participants is required" {
        t.Errorf("AssignSecretSanta() returned unexpected error: %v", err)
    }
}

func TestAssignSecretSanta(t *testing.T) {
	participants := []Participant{
		{ID: 1, Name: "Alice", Email: "alice@example.com"},
		{ID: 2, Name: "Bob", Email: "bob@example.com"},
		{ID: 3, Name: "Charlie", Email: "charlie@example.com"},
		{ID: 4, Name: "Dean", Email: "dean@example.com"},
		{ID: 5, Name: "Earl", Email: "earl@example.com"},
	}

	assignments, err := AssignSecretSanta(participants)
	if err != nil {
		t.Errorf("AssignSecretSanta() error = %v", err)
		return
	}

	sortedAssignments := make([]Assignment, len(assignments))
    copy(sortedAssignments, assignments)

    // Sort the copy by Participant.Name
    sort.Slice(sortedAssignments, func(i, j int) bool {
        return sortedAssignments[i].Participant.Name < sortedAssignments[j].Participant.Name
    })

	// debug: print the assignments in an easy to read way,
	for _, assignment := range sortedAssignments {
		t.Logf(`"%s" -> "%s"`, assignment.Participant.Name, assignment.GifteeName)
	}

	if len(assignments) != len(participants) {
		t.Errorf("Expected %d assignments, got %d", len(participants), len(assignments))
	}

	// 1. create a map from participant names, with int 0 def value
	gifteeCount := make(map[string]int)
	for _, assignment := range assignments {
		// 2. for each assert that Assignment.Participant.Name != Assignment.GifteeName
		if assignment.Participant.Name == assignment.GifteeName {
            t.Errorf("Participant %s is assigned to gift themselves", assignment.Participant.Name)
        }
		// 3. increment the map from step 1 for the given Assignment.GifteeName
        gifteeCount[assignment.GifteeName]++
	}
	
	// 4. Assert as many giftees as participants
	if len(gifteeCount) != len(participants) {
		t.Errorf("Expected %d giftees, got %d", len(participants), len(gifteeCount))
	}

	// 5. Assert that each participant got gifted exactly once (the map has the value 1 for all names)
	for _, participant := range participants {
        if gifteeCount[participant.Name] != 1 {
            t.Errorf("Participant %s is gifted %d times, expected exactly 1", participant.Name, gifteeCount[participant.Name])
        }
    }
}