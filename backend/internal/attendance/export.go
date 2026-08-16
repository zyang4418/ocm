package attendance

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/xuri/excelize/v2"
	"ocm-backend/internal/httpx"
	"ocm-backend/internal/xlsx"
)

// exportCheckin streams one checkin's record list as an xlsx download. The
// roster's derived-absent rows are included, so the missing list is readable
// straight from the file.
func (h *Handler) exportCheckin(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.RespondError(w, http.StatusBadRequest, "invalid checkin id")
		return
	}
	if _, err := h.store.GetCheckin(r.Context(), id); err != nil {
		if errors.Is(err, ErrCheckinNotFound) {
			httpx.RespondError(w, http.StatusNotFound, "checkin not found")
			return
		}
		httpx.Error500(w, r, "could not load checkin", err)
		return
	}
	list, err := h.store.ListRecordsAll(r.Context(), id)
	if err != nil {
		httpx.Error500(w, r, "could not list checkin records", err)
		return
	}
	headers := []string{"姓名", "学号", "行政班", "状态", "签到时间", "是否名单内"}
	rows := make([][]any, 0, len(list))
	for _, v := range list {
		checkedAt := ""
		if v.CheckedAt != nil {
			checkedAt = v.CheckedAt.Format("2006-01-02 15:04")
		}
		inRoster := "是"
		if !v.InRoster {
			inRoster = "否"
		}
		rows = append(rows, []any{v.DisplayName, v.StudentNo, v.AdminClass,
			statusLabel(v.Status), checkedAt, inRoster})
	}
	if err := xlsx.WriteExport(w, fmt.Sprintf("checkin-%d.xlsx", id), "签到明细", headers, rows); err != nil {
		httpx.Error500(w, r, "could not export checkin records", err)
	}
}

// exportOfferingReport streams the L2 semester report for one offering: a
// per-checkin summary sheet plus a student × checkin matrix sheet.
func (h *Handler) exportOfferingReport(w http.ResponseWriter, r *http.Request) {
	offeringID := queryInt(r.URL.Query(), "offering_id")
	if offeringID <= 0 {
		httpx.RespondError(w, http.StatusBadRequest, "offering_id is required")
		return
	}
	sum, err := h.store.OfferingSummary(r.Context(), offeringID)
	if errors.Is(err, ErrCheckinNotFound) {
		httpx.RespondError(w, http.StatusNotFound, "offering not found")
		return
	}
	if err != nil {
		httpx.Error500(w, r, "could not build attendance summary", err)
		return
	}
	displayName := fmt.Sprintf("%s-%s-考勤汇总.xlsx", sum.CourseName, sum.Semester)
	if displayName == "-考勤汇总.xlsx" {
		displayName = fmt.Sprintf("开课%d-考勤汇总.xlsx", offeringID)
	}
	asciiName := fmt.Sprintf("attendance-%d.xlsx", offeringID)
	err = xlsx.WriteCustom(w, asciiName, displayName, func(f *excelize.File) error {
		return buildReportSheets(f, sum)
	})
	if err != nil {
		httpx.Error500(w, r, "could not export attendance report", err)
	}
}

// buildReportSheets renders both sheets into f. Sheet 1 summarizes each
// checkin; sheet 2 is the student × checkin matrix with per-status subtotals.
func buildReportSheets(f *excelize.File, sum OfferingSummary) error {
	summarySheet := "汇总统计"
	detailSheet := "签到明细"
	if err := f.SetSheetName("Sheet1", summarySheet); err != nil {
		return fmt.Errorf("rename summary sheet: %w", err)
	}
	if _, err := f.NewSheet(detailSheet); err != nil {
		return fmt.Errorf("create detail sheet: %w", err)
	}

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#E8E8E8"}},
	})
	if err != nil {
		return fmt.Errorf("create header style: %w", err)
	}

	// Sheet 1: one row per checkin.
	headers := []string{"日期", "标题", "应到", "实到", "出勤", "迟到", "缺勤", "请假", "出勤率"}
	for i, hd := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(summarySheet, cell, hd); err != nil {
			return fmt.Errorf("set summary header: %w", err)
		}
		if err := f.SetCellStyle(summarySheet, cell, cell, headerStyle); err != nil {
			return fmt.Errorf("style summary header: %w", err)
		}
	}
	for r, c := range sum.Checkins {
		attended := c.Counts.Present + c.Counts.Late
		rate := "-"
		if c.Counts.Expected > 0 {
			rate = fmt.Sprintf("%.1f%%", float64(attended)/float64(c.Counts.Expected)*100)
		}
		row := []any{c.StartsAt.Format("2006-01-02"), c.Title, c.Counts.Expected, attended,
			c.Counts.Present, c.Counts.Late, c.Counts.Absent, c.Counts.Leave, rate}
		for i, v := range row {
			cell, _ := excelize.CoordinatesToCellName(i+1, r+2)
			if err := f.SetCellValue(summarySheet, cell, v); err != nil {
				return fmt.Errorf("set summary row: %w", err)
			}
		}
	}
	if err := f.SetPanes(summarySheet, &excelize.Panes{
		Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	}); err != nil {
		return fmt.Errorf("freeze summary header: %w", err)
	}

	// Sheet 2: student × checkin matrix.
	subtotals := []string{StatusPresent, StatusLate, StatusAbsent, StatusLeave}
	matrixHeaders := []string{"姓名", "学号", "行政班"}
	for _, c := range sum.Checkins {
		matrixHeaders = append(matrixHeaders, c.StartsAt.Format("2006-01-02"))
	}
	for _, s := range subtotals {
		matrixHeaders = append(matrixHeaders, statusLabel(s))
	}
	for i, hd := range matrixHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(detailSheet, cell, hd); err != nil {
			return fmt.Errorf("set detail header: %w", err)
		}
		if err := f.SetCellStyle(detailSheet, cell, cell, headerStyle); err != nil {
			return fmt.Errorf("style detail header: %w", err)
		}
	}
	for r, row := range sum.Rows {
		cells := []any{row.DisplayName, row.StudentNo, row.AdminClass}
		for _, c := range sum.Checkins {
			cells = append(cells, statusLabel(row.Records[c.ID]))
		}
		for _, s := range subtotals {
			cells = append(cells, row.Totals[s])
		}
		for i, v := range cells {
			cell, _ := excelize.CoordinatesToCellName(i+1, r+2)
			if err := f.SetCellValue(detailSheet, cell, v); err != nil {
				return fmt.Errorf("set detail cell: %w", err)
			}
		}
	}
	if err := f.SetPanes(detailSheet, &excelize.Panes{
		Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	}); err != nil {
		return fmt.Errorf("freeze detail header: %w", err)
	}
	return nil
}
