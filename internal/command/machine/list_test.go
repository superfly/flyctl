package machine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
)

type machineListPage struct {
	response *flaps.ListMachinesResponse
	err      error
}

type fakeMachinePageLister struct {
	pages  map[string]machineListPage
	opts   []*flaps.ListMachinesOpts
	appIDs []string
}

func (f *fakeMachinePageLister) ListMachines(_ context.Context, appName string, opts *flaps.ListMachinesOpts) (*flaps.ListMachinesResponse, error) {
	f.appIDs = append(f.appIDs, appName)
	f.opts = append(f.opts, opts)
	page := f.pages[opts.Cursor]
	return page.response, page.err
}

func TestMachineListNavigationPager(t *testing.T) {
	command := machineListNavigationPager(machineListNavigation{hasNext: true, hasPrev: true, page: 2})
	for _, want := range []string{"n quit n", "p quit p", "q quit q", "page 2", "[n] next", "[p] previous", "[q] quit"} {
		assert.Contains(t, command, want)
	}

	firstPageCommand := machineListNavigationPager(machineListNavigation{hasNext: true, page: 1})
	assert.NotContains(t, firstPageCommand, "[p] previous")
	assert.NotContains(t, firstPageCommand, "p quit p")

	lastPageCommand := machineListNavigationPager(machineListNavigation{hasPrev: true, page: 3})
	assert.NotContains(t, lastPageCommand, "n quit n")
}

func TestMachineListActionFromExitCode(t *testing.T) {
	for _, test := range []struct {
		exitCode int
		want     machineListAction
	}{
		{exitCode: int('n'), want: machineListNextPage},
		{exitCode: int('p'), want: machineListPrevPage},
		{exitCode: int('q'), want: machineListQuit},
		{exitCode: 0, want: machineListQuit},
	} {
		assert.Equal(t, test.want, machineListActionFromExitCode(test.exitCode))
	}
}

func TestListMachinePagePaginatesAndDeduplicates(t *testing.T) {
	client := &fakeMachinePageLister{pages: map[string]machineListPage{
		"": {
			response: &flaps.ListMachinesResponse{
				Machines:   []*fly.Machine{{ID: "machine-1"}, {ID: "machine-2"}},
				NextCursor: "page-2",
			},
		},
		"page-2": {
			response: &flaps.ListMachinesResponse{
				Machines: []*fly.Machine{{ID: "machine-2"}, {ID: "machine-3"}},
			},
		},
	}}

	machines, nextCursor, err := loadMachineDisplayPage(t.Context(), client, "test-app", 0, "", map[string]struct{}{}, map[string]struct{}{})
	require.NoError(t, err)
	assert.Empty(t, nextCursor)

	require.Len(t, machines, 3)
	assert.Equal(t, "machine-1", machines[0].ID)
	assert.Equal(t, "machine-2", machines[1].ID)
	assert.Equal(t, "machine-3", machines[2].ID)
	require.Len(t, client.opts, 2)
	assert.Equal(t, 500, client.opts[0].Limit)
	assert.Equal(t, "page-2", client.opts[1].Cursor)
	assert.Equal(t, []string{"test-app", "test-app"}, client.appIDs)
}

func TestListMachinePageRejectsRepeatedCursor(t *testing.T) {
	client := &fakeMachinePageLister{pages: map[string]machineListPage{
		"":       {response: &flaps.ListMachinesResponse{NextCursor: "page-2"}},
		"page-2": {response: &flaps.ListMachinesResponse{NextCursor: "page-2"}},
	}}

	_, _, err := loadMachineDisplayPage(context.Background(), client, "test-app", 0, "", map[string]struct{}{}, map[string]struct{}{})
	require.EqualError(t, err, "Machines API returned a repeated pagination cursor")
}

func TestListMachinePageStopsAtLimit(t *testing.T) {
	client := &fakeMachinePageLister{pages: map[string]machineListPage{
		"": {
			response: &flaps.ListMachinesResponse{
				Machines:   []*fly.Machine{{ID: "machine-1"}, {ID: "machine-2"}},
				NextCursor: "page-2",
			},
		},
		"page-2": {
			response: &flaps.ListMachinesResponse{
				Machines:   []*fly.Machine{{ID: "machine-3"}},
				NextCursor: "page-3",
			},
		},
	}}

	machines, nextCursor, err := loadMachineDisplayPage(t.Context(), client, "test-app", 3, "", map[string]struct{}{}, map[string]struct{}{})
	require.NoError(t, err)
	require.Len(t, machines, 3)
	require.Len(t, client.opts, 2)
	assert.Equal(t, 3, client.opts[0].Limit)
	assert.Equal(t, 1, client.opts[1].Limit)
	assert.Equal(t, "page-3", nextCursor)
}

func TestListMachinePageContinuesFromCursor(t *testing.T) {
	client := &fakeMachinePageLister{pages: map[string]machineListPage{
		"": {
			response: &flaps.ListMachinesResponse{
				Machines:   []*fly.Machine{{ID: "machine-1"}, {ID: "machine-2"}},
				NextCursor: "page-2",
			},
		},
		"page-2": {
			response: &flaps.ListMachinesResponse{
				Machines: []*fly.Machine{{ID: "machine-3"}, {ID: "machine-4"}},
			},
		},
	}}
	seenMachines := map[string]struct{}{}
	seenCursors := map[string]struct{}{}

	firstPage, nextCursor, err := loadMachineDisplayPage(t.Context(), client, "test-app", 2, "", seenMachines, seenCursors)
	require.NoError(t, err)
	secondPage, nextCursor, err := loadMachineDisplayPage(t.Context(), client, "test-app", 2, nextCursor, seenMachines, seenCursors)
	require.NoError(t, err)

	assert.Len(t, firstPage, 2)
	assert.Len(t, secondPage, 2)
	assert.Empty(t, nextCursor)
}
