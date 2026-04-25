package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"blackbox/server/internal/models"
	"blackbox/shared/types"
	"github.com/go-chi/chi/v5"
	"github.com/phpdave11/gofpdf"
	"gorm.io/gorm"
)

type reportData struct {
	Incident    models.Incident
	Entries     []reportEntry
	AIAnalysis  string
	AIModel     string
	NodeNames   []string
	Services    []string
	GeneratedAt time.Time
}

type reportEntry struct {
	Link  models.IncidentEntry
	Entry types.Entry
}

func DownloadIncidentReport(database *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var incident models.Incident
		if err := database.First(&incident, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				writeError(w, http.StatusNotFound, "incident not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to fetch incident")
			return
		}

		if incident.Status != "resolved" {
			writeError(w, http.StatusBadRequest, "incident is not resolved")
			return
		}

		var links []models.IncidentEntry
		if err := database.Where("incident_id = ?", id).Order("score DESC").Find(&links).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch incident entries")
			return
		}

		reportEntries := make([]reportEntry, 0, len(links))
		for _, link := range links {
			var entry types.Entry
			if err := database.First(&entry, "id = ?", link.EntryID).Error; err != nil {
				log.Printf("DownloadIncidentReport missing entry for incident %s entry %s role %s: %v", link.IncidentID, link.EntryID, link.Role, err)
				continue
			}
			reportEntries = append(reportEntries, reportEntry{Link: link, Entry: entry})
		}
		sort.Slice(reportEntries, func(i, j int) bool {
			return reportEntries[i].Entry.Timestamp.Before(reportEntries[j].Entry.Timestamp)
		})

		aiAnalysis, aiModel := incidentAIFields(incident.Metadata)
		data := reportData{
			Incident:    incident,
			Entries:     reportEntries,
			AIAnalysis:  aiAnalysis,
			AIModel:     aiModel,
			NodeNames:   decodeStringList(incident.NodeNames),
			Services:    decodeStringList(incident.Services),
			GeneratedAt: time.Now().UTC(),
		}

		pdfBytes, err := generateIncidentPDF(data)
		if err != nil {
			log.Printf("DownloadIncidentReport PDF generation failed for incident %s: %v", id, err)
			writeError(w, http.StatusInternalServerError, "failed to generate PDF")
			return
		}

		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="incident-%s-report.pdf"`, data.Incident.ID))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pdfBytes)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pdfBytes)
	}
}

func incidentAIFields(metadata string) (string, string) {
	var meta map[string]interface{}
	if metadata == "" {
		return "", ""
	}
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		return "", ""
	}
	analysis, _ := meta["ai_analysis"].(string)
	model, _ := meta["ai_model"].(string)
	return strings.TrimSpace(analysis), strings.TrimSpace(model)
}

func decodeStringList(value string) []string {
	var items []string
	if value != "" {
		_ = json.Unmarshal([]byte(value), &items)
	}
	if items == nil {
		return []string{}
	}
	return items
}

func generateIncidentPDF(data reportData) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 20, 20)

	// Fill each page with black background before any content is drawn.
	pdf.SetHeaderFunc(func() {
		pdf.SetFillColor(0, 0, 0)
		pdf.Rect(0, 0, 210, 297, "F")
		pdf.SetXY(20, 20)
	})

	pdf.AddPage()

	pdfHeader(pdf, data)
	pdfRule(pdf)
	pdfOverview(pdf, data)
	pdfRule(pdf)
	pdfAISection(pdf, data)
	pdfEventChain(pdf, data.Entries)
	pdfRule(pdf)
	pdfFooter(pdf, data.GeneratedAt)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// pdfRed sets the text color to the report accent red.
func pdfRed(pdf *gofpdf.Fpdf) { pdf.SetTextColor(0xCC, 0x22, 0x22) }

// pdfWhite sets the text color to near-white for body content.
func pdfWhite(pdf *gofpdf.Fpdf) { pdf.SetTextColor(0xE8, 0xE8, 0xE8) }

// pdfDim sets the text color to a dimmer gray for secondary content.
func pdfDim(pdf *gofpdf.Fpdf) { pdf.SetTextColor(0xA0, 0xA0, 0xA0) }

func pdfHeader(pdf *gofpdf.Fpdf, data reportData) {
	pdf.SetFont("Courier", "", 12)
	pdfRed(pdf)

	label := "BLACKBOX INCIDENT REPORT"
	idStr := fmt.Sprintf("#%s", data.Incident.ID)
	pageW, _, _ := pdf.PageSize(0)
	usableW := pageW - 40

	pdf.CellFormat(usableW/2, 7, label, "", 0, "L", false, 0, "")
	pdf.CellFormat(usableW/2, 7, idStr, "", 1, "R", false, 0, "")

	pdf.SetFont("Courier", "B", 16)
	pdfWhite(pdf)
	pdf.MultiCell(0, 9, sanitizePDFString(data.Incident.Title), "", "L", false)
	pdf.Ln(2)
}

func pdfOverview(pdf *gofpdf.Fpdf, data reportData) {
	inc := data.Incident
	pdf.SetFont("Courier", "", 11)
	pdfRed(pdf)
	pdf.CellFormat(0, 6, "OVERVIEW", "", 1, "L", false, 0, "")
	pdf.Ln(1)

	keyW := 34.0
	row := func(key, value string) {
		pdf.SetFont("Courier", "", 11)
		pdfRed(pdf)
		pdf.CellFormat(keyW, 6, key, "", 0, "L", false, 0, "")
		pdfWhite(pdf)
		pdf.MultiCell(0, 6, sanitizePDFString(value), "", "L", false)
	}

	row("Status:", strings.ToUpper(inc.Status))
	row("Confidence:", strings.ToUpper(inc.Confidence))
	row("Nodes:", joinOrDash(data.NodeNames))
	row("Services:", joinOrDash(data.Services))
	row("Opened (UTC):", inc.OpenedAt.UTC().Format("2006-01-02 15:04:05"))

	resolvedVal := "-"
	durationVal := "-"
	if inc.ResolvedAt != nil {
		resolvedVal = inc.ResolvedAt.UTC().Format("2006-01-02 15:04:05")
		durationVal = formatDuration(inc.OpenedAt, *inc.ResolvedAt)
	}
	row("Resolved (UTC):", resolvedVal)
	row("Duration:", durationVal)
	pdf.Ln(2)
}

func joinOrDash(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, ", ")
}

func pdfAISection(pdf *gofpdf.Fpdf, data reportData) {
	if data.AIAnalysis == "" {
		return
	}

	pdf.SetFont("Courier", "", 11)
	pdfRed(pdf)
	label := "AI ANALYSIS"
	if data.AIModel != "" {
		label = fmt.Sprintf("AI ANALYSIS  [%s]", data.AIModel)
	}
	pdf.CellFormat(0, 6, sanitizePDFString(label), "", 1, "L", false, 0, "")
	pdf.Ln(1)

	pdf.SetFont("Courier", "", 11)
	pdfWhite(pdf)
	pdf.MultiCell(0, 6, sanitizePDFString(data.AIAnalysis), "", "L", false)
	pdf.Ln(2)
	pdfRule(pdf)
}

func pdfEventChain(pdf *gofpdf.Fpdf, entries []reportEntry) {
	pdf.SetFont("Courier", "", 11)
	pdfRed(pdf)
	pdf.CellFormat(0, 6, fmt.Sprintf("EVENT CHAIN  (%d entries)", len(entries)), "", 1, "L", false, 0, "")
	pdf.Ln(1)

	pdf.SetFont("Courier", "", 10)
	pdfRed(pdf)
	pageW, _, _ := pdf.PageSize(0)
	usableW := pageW - 40
	colRole := 24.0
	colTS := 50.0
	colSrc := 24.0
	colSvc := 32.0
	colEvent := usableW - colRole - colTS - colSrc - colSvc

	pdf.CellFormat(colRole, 5, "ROLE", "", 0, "L", false, 0, "")
	pdf.CellFormat(colTS, 5, "TIMESTAMP", "", 0, "L", false, 0, "")
	pdf.CellFormat(colSrc, 5, "SOURCE", "", 0, "L", false, 0, "")
	pdf.CellFormat(colSvc, 5, "SERVICE", "", 0, "L", false, 0, "")
	pdf.CellFormat(colEvent, 5, "EVENT", "", 1, "L", false, 0, "")

	for _, re := range entries {
		link := re.Link
		entry := re.Entry
		pdf.SetFont("Courier", "", 10)
		pdfWhite(pdf)
		pdf.CellFormat(colRole, 5, roleLabel(link.Role), "", 0, "L", false, 0, "")
		pdf.CellFormat(colTS, 5, entry.Timestamp.UTC().Format("2006-01-02 15:04:05"), "", 0, "L", false, 0, "")
		pdf.CellFormat(colSrc, 5, sanitizePDFString(entry.Source), "", 0, "L", false, 0, "")
		pdf.CellFormat(colSvc, 5, sanitizePDFString(entry.Service), "", 0, "L", false, 0, "")
		pdf.CellFormat(colEvent, 5, sanitizePDFString(entry.Event), "", 1, "L", false, 0, "")

		if entry.Content != "" {
			content := sanitizePDFString(truncateRunes(entry.Content, 120))
			pdf.SetX(26)
			pdf.SetFont("Courier", "", 10)
			pdfDim(pdf)
			pdf.MultiCell(usableW-6, 4, content, "", "L", false)
		}

		if link.Role == "ai_cause" && link.Reason != "" {
			reason := sanitizePDFString("AI: " + truncateRunes(link.Reason, 120))
			pdf.SetX(26)
			pdf.SetFont("Courier", "I", 10)
			pdfDim(pdf)
			pdf.MultiCell(usableW-6, 4, reason, "", "L", false)
		}
	}
	pdf.Ln(2)
}

func pdfFooter(pdf *gofpdf.Fpdf, generatedAt time.Time) {
	pdf.SetFont("Courier", "", 10)
	pdfDim(pdf)
	pdf.CellFormat(0, 6, fmt.Sprintf("Generated by Blackbox on %s", generatedAt.UTC().Format("2006-01-02")), "", 1, "C", false, 0, "")
}

func pdfRule(pdf *gofpdf.Fpdf) {
	pdf.Ln(2)
	pdf.SetDrawColor(0x44, 0x08, 0x08)
	pdf.SetLineWidth(0.3)
	x := pdf.GetX()
	y := pdf.GetY()
	pageW, _, _ := pdf.PageSize(0)
	pdf.Line(20, y, pageW-20, y)
	pdf.SetXY(x, y+3)
}

func roleLabel(role string) string {
	switch role {
	case "trigger":
		return "TRIGGER"
	case "cause":
		return "CAUSE"
	case "evidence":
		return "EVIDENCE"
	case "recovery":
		return "RECOVERY"
	case "ai_cause":
		return "AI CAUSE"
	default:
		return strings.ToUpper(role)
	}
}

func formatDuration(opened time.Time, resolved time.Time) string {
	secs := int(math.Round(resolved.Sub(opened).Seconds()))
	if secs < 0 {
		secs = 0
	}
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	if secs < 3600 {
		return fmt.Sprintf("%dm %ds", secs/60, secs%60)
	}
	return fmt.Sprintf("%dh %dm", secs/3600, (secs%3600)/60)
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "..."
}

func sanitizePDFString(s string) string {
	replacer := strings.NewReplacer(
		"‘", "'", "’", "'", // curly single quotes
		"“", "\"", "”", "\"", // curly double quotes
		"–", "-", "—", "--", // en/em dash
		"…", "...", // ellipsis
		" ", " ", // non-breaking space
	)
	// Strip remaining non-Latin-1 chars
	var b strings.Builder
	for _, r := range replacer.Replace(s) {
		if r <= 0xFF {
			b.WriteRune(r)
		} else {
			b.WriteRune('?')
		}
	}
	return b.String()
}
