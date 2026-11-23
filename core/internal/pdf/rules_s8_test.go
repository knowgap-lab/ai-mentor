package pdf_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/knowgap-lab/ai-mentor/core/internal/gotenbergtest"
)

func TestWB_R28_S8_SheetPdf(t *testing.T) {
	stub := gotenbergtest.Serve(t)
	h := s8NewHarness(t, stub.URL)
	body := s8PDF(t, h.sheet(s8SheetID))
	html := s8HTML(t, stub)
	s8NoExternalURL(t, html)
	if !strings.Contains(html, s8Statement) {
		t.Fatal("в листке нет условия")
	}
	if !bytes.HasPrefix(body, []byte("%PDF-")) {
		t.Fatal("ответ не начинается с %PDF-")
	}
}

func TestWB_R28_S8_FormPdf(t *testing.T) {
	stub := gotenbergtest.Serve(t)
	h := s8NewHarness(t, stub.URL)
	h.as(s8TutorTG, "tutor")
	_ = s8PDF(t, h.form())
	html := s8HTML(t, stub)
	s8NoExternalURL(t, html)
	if !strings.Contains(html, s8FormTitle) {
		t.Fatal("в бланке нет названия теста")
	}
	if strings.Contains(html, s8FormKey) || strings.Contains(html, s8SheetKey) {
		t.Fatal("в бланке есть ключ")
	}
}

func TestWB_R4_S8_SheetOmitsKey(t *testing.T) {
	stub := gotenbergtest.Serve(t)
	h := s8NewHarness(t, stub.URL)
	body := s8PDF(t, h.sheet(s8SheetID))
	html := s8HTML(t, stub)
	if strings.Contains(html, s8SheetKey) || bytes.Contains(body, []byte(s8SheetKey)) {
		t.Fatal("в тексте листка есть ключ")
	}
	if !strings.Contains(html, s8Statement) {
		t.Fatal("в листке нет условия")
	}
}

func TestWB_R29_S8_TutorPdfOmitsKey(t *testing.T) {
	stub := gotenbergtest.Serve(t)
	h := s8NewHarness(t, stub.URL)
	h.as(s8TutorTG, "tutor")
	sheet := s8PDF(t, h.sheet(s8SheetID))
	sheetHTML := s8HTML(t, stub)
	if strings.Contains(sheetHTML, s8SheetKey) || bytes.Contains(sheet, []byte(s8SheetKey)) {
		t.Fatal("в листке репетитора есть ключ")
	}
	before := len(stub.Bodies())
	form := s8PDF(t, h.form())
	if bytes.Contains(form, []byte(s8FormKey)) || bytes.Contains(form, []byte(s8SheetKey)) {
		t.Fatal("в бланке репетитора есть ключ")
	}
	for _, body := range stub.Bodies()[before:] {
		if strings.Contains(string(body), s8FormKey) || strings.Contains(string(body), s8SheetKey) {
			t.Fatal("в HTML бланка есть ключ")
		}
	}
}

func TestS8_ForeignSheet(t *testing.T) {
	stub := gotenbergtest.Serve(t)
	h := s8NewHarness(t, stub.URL)
	h.expectCode(h.sheet(s8OtherSheet), http.StatusForbidden, "forbidden")
	h.expectCode(h.sheet("s8-sheet-missing"), http.StatusForbidden, "forbidden")
	h.as(s8TutorTG, "tutor")
	h.expectCode(h.sheet(s8OtherSheet), http.StatusForbidden, "forbidden")
	h.expectCode(h.sheet("s8-sheet-missing"), http.StatusForbidden, "forbidden")
	h.token = ""
	h.expectCode(h.sheet(s8SheetID), http.StatusUnauthorized, "unauthorized")
	h.expectCode(h.form(), http.StatusUnauthorized, "unauthorized")
}
