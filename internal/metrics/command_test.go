package metrics

import "testing"

func TestSetAppName(t *testing.T) {
	t.Cleanup(func() { appName = "" })

	SetAppName("my-app")
	if appName != "my-app" {
		t.Fatalf("appName = %q, want %q", appName, "my-app")
	}

	// Preparers run in sequence and a later one resolving no app must not
	// erase a name an earlier one already reported.
	SetAppName("")
	if appName != "my-app" {
		t.Fatalf("appName = %q after empty name, want it left alone", appName)
	}
}

func TestSetAppOrgIDs(t *testing.T) {
	t.Cleanup(func() { appID, orgID = "", "" })

	SetAppOrgIDs("123", "456")
	if appID != "123" || orgID != "456" {
		t.Fatalf("appID, orgID = %q, %q; want %q, %q", appID, orgID, "123", "456")
	}

	// A caller that only knows one of the two leaves the other alone.
	SetAppOrgIDs("", "789")
	if appID != "123" || orgID != "789" {
		t.Fatalf("appID, orgID = %q, %q; want %q, %q", appID, orgID, "123", "789")
	}
}

// The app name and the token-scoped IDs are independent signals: a command can
// report a name while its tokens span too many orgs to name one, and a command
// with no app at all can still be attributed by an org-scoped token.
func TestAppNameAndIDsAreIndependent(t *testing.T) {
	t.Cleanup(func() { appName, appID, orgID = "", "", "" })

	SetAppName("my-app")
	if appID != "" || orgID != "" {
		t.Fatalf("SetAppName touched IDs: appID=%q orgID=%q", appID, orgID)
	}

	SetAppOrgIDs("123", "456")
	if appName != "my-app" {
		t.Fatalf("SetAppOrgIDs touched appName: %q", appName)
	}
}
