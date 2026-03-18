package tables

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestContacts_SaveFindDelete(t *testing.T) {
	db := testDB(t)
	c := NewContacts(db)

	contact := &Contact{UserID: 100, ChatID: 200, Phone: "8(903)111-22-33"}
	inserted, err := c.Save(contact)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !inserted {
		t.Error("Save: expected inserted true")
	}

	found, err := c.Find("100")
	if err != nil {
		t.Fatalf("Find by userID: %v", err)
	}
	if found == nil || found.Phone != "79031112233" {
		t.Errorf("Find: got %+v, expected normalized phone 79031112233", found)
	}

	foundByPhone, err := c.Find("79031112233")
	if err != nil || foundByPhone == nil {
		t.Fatalf("Find by phone: err=%v contact=%v", err, foundByPhone)
	}

	// Update
	contact.ChatID = 201
	inserted2, err := c.Save(contact)
	if err != nil {
		t.Fatalf("Save update: %v", err)
	}
	if inserted2 {
		t.Error("Save update: expected inserted false")
	}
	found2, _ := c.Find("100")
	if found2.ChatID != 201 {
		t.Errorf("after update ChatID: got %d", found2.ChatID)
	}

	deleted, err := c.Delete(100)
	if err != nil || !deleted {
		t.Fatalf("Delete: err=%v deleted=%v", err, deleted)
	}
	found3, _ := c.Find("100")
	if found3 != nil {
		t.Error("Find after delete: expected nil")
	}
}

func TestGroupChats_SaveFindByChatIDAllSetMenuSentDelete(t *testing.T) {
	db := testDB(t)
	gc := NewGroupChats(db)

	chat := &GroupChat{ChatID: 1, Title: "Test Group", MenuSent: false}
	inserted, err := gc.Save(chat)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !inserted {
		t.Error("Save: expected inserted true")
	}

	found, err := gc.FindByChatID(1)
	if err != nil {
		t.Fatalf("FindByChatID: %v", err)
	}
	if found == nil || found.Title != "Test Group" || found.MenuSent {
		t.Errorf("FindByChatID: got %+v", found)
	}

	all, err := gc.All()
	if err != nil || len(all) != 1 {
		t.Fatalf("All: err=%v len=%d", err, len(all))
	}

	if err := gc.SetMenuSent(1); err != nil {
		t.Fatalf("SetMenuSent: %v", err)
	}
	found2, _ := gc.FindByChatID(1)
	if found2 == nil || !found2.MenuSent {
		t.Errorf("after SetMenuSent: got %+v", found2)
	}

	if err := gc.DeleteByChatID(1); err != nil {
		t.Fatalf("DeleteByChatID: %v", err)
	}
	found3, _ := gc.FindByChatID(1)
	if found3 != nil {
		t.Error("FindByChatID after delete: expected nil")
	}
}

func TestGroupChats_FindByTitle(t *testing.T) {
	db := testDB(t)
	gc := NewGroupChats(db)
	_, _ = gc.Save(&GroupChat{ChatID: 10, Title: "Alpha", MenuSent: false})

	found, err := gc.FindByTitle("Alpha")
	if err != nil || found == nil || found.ChatID != 10 {
		t.Fatalf("FindByTitle: err=%v found=%+v", err, found)
	}

	notFound, _ := gc.FindByTitle("Nonexistent")
	if notFound != nil {
		t.Error("FindByTitle nonexistent: expected nil")
	}
}
