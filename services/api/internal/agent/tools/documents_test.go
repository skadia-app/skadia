package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/google/uuid"
)

func TestSearchDocumentsRequiresDocumentTextPreference(t *testing.T) {
	fx := newGearToolFixture(t)
	insertSearchDocument(t, fx, "receipt.pdf", "Neue Kette GX Eagle", "Rechnung Kette")

	got, err := executeSearchDocuments(context.Background(), &Deps{DB: fx.db}, fx.userID, json.RawMessage(`{"query":"Kette"}`))
	if err != nil {
		t.Fatalf("executeSearchDocuments: %v", err)
	}
	result := got.(map[string]any)
	if result["count"] != 0 {
		t.Fatalf("count = %#v, want 0 while disabled", result["count"])
	}
	if result["reason"] != "document_text_not_enabled" {
		t.Fatalf("reason = %#v, want document_text_not_enabled", result["reason"])
	}
}

func TestSearchDocumentsFindsExtractedMarkdown(t *testing.T) {
	fx := newGearToolFixture(t)
	if _, err := fx.db.UpsertChatPrefs(context.Background(), fx.userID, database.ChatUserPreferences{
		ExposeDocumentExtractedData: true,
	}); err != nil {
		t.Fatalf("upsert chat prefs: %v", err)
	}
	insertSearchDocument(t, fx, "receipt.pdf", "## Rechnung\n\nNeue GX Fahrradkette montiert.", "OCR Text")
	insertSearchDocument(t, fx, "manual.pdf", "Handbuch ohne Treffer", "Manual Text")

	got, err := executeSearchDocuments(context.Background(), &Deps{DB: fx.db}, fx.userID, json.RawMessage(`{"query":"GX Fahrradkette","limit":5}`))
	if err != nil {
		t.Fatalf("executeSearchDocuments: %v", err)
	}
	result := got.(map[string]any)
	if result["count"] != 1 {
		t.Fatalf("count = %#v, want 1", result["count"])
	}
	docs := result["documents"].([]map[string]any)
	if docs[0]["original_filename"] != "receipt.pdf" {
		t.Fatalf("filename = %#v, want receipt.pdf", docs[0]["original_filename"])
	}
	if !strings.Contains(docs[0]["snippet"].(string), "GX Fahrradkette") {
		t.Fatalf("snippet = %#v, want markdown hit", docs[0]["snippet"])
	}
	if docs[0]["gear_id"] != fx.gearID {
		t.Fatalf("gear_id = %#v, want fixture gear", docs[0]["gear_id"])
	}
}

func TestSearchDocumentsFiltersByGearID(t *testing.T) {
	fx := newGearToolFixture(t)
	if _, err := fx.db.UpsertChatPrefs(context.Background(), fx.userID, database.ChatUserPreferences{
		ExposeDocumentExtractedData: true,
	}); err != nil {
		t.Fatalf("upsert chat prefs: %v", err)
	}
	otherGearID := fx.insertGear(t, "Other Bike", "bike", "bike", false)
	insertSearchDocumentForGear(t, fx, fx.gearID, "fixture.pdf", "Garantie Rahmen", "")
	insertSearchDocumentForGear(t, fx, otherGearID, "other.pdf", "Garantie Laufrad", "")

	got, err := executeSearchDocuments(context.Background(), &Deps{DB: fx.db}, fx.userID, json.RawMessage(`{"query":"Garantie","gear_id":"`+fx.gearID+`"}`))
	if err != nil {
		t.Fatalf("executeSearchDocuments: %v", err)
	}
	docs := got.(map[string]any)["documents"].([]map[string]any)
	if len(docs) != 1 {
		t.Fatalf("docs len = %d, want 1", len(docs))
	}
	if docs[0]["original_filename"] != "fixture.pdf" {
		t.Fatalf("filename = %#v, want fixture.pdf", docs[0]["original_filename"])
	}
}

func insertSearchDocument(t *testing.T, fx gearToolFixture, filename, markdown, text string) {
	t.Helper()
	insertSearchDocumentForGear(t, fx, fx.gearID, filename, markdown, text)
}

func insertSearchDocumentForGear(t *testing.T, fx gearToolFixture, gearID, filename, markdown, text string) {
	t.Helper()
	_, err := fx.pool.Exec(context.Background(), `
		INSERT INTO gear_documents
			(id, user_id, gear_id, doc_type, original_filename, content_type, file_size_bytes,
			 extracted_markdown, extracted_text, extracted_data, processing_status)
		VALUES
			($1, $2, $3, 'receipt', $4, 'application/pdf', 123,
			 $5, $6, '{"vendor":"Shop"}'::jsonb, 'processed')`,
		uuid.NewString(), fx.userID, gearID, filename, markdown, text,
	)
	if err != nil {
		t.Fatalf("insert search document: %v", err)
	}
}
