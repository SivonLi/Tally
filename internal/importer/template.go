package importer

import (
	"fmt"

	"github.com/xuri/excelize/v2"

	"tally/internal/excel"
	"tally/internal/store"
)

// WriteTemplate 写出一个数据项的导入模板。
func WriteTemplate(kind string, subjects []store.Subject, path string) error {
	if path == "" {
		return fmt.Errorf("模板保存路径不能为空")
	}
	headers := TemplateHeaders(kind, subjects)
	if len(headers) == 0 {
		return fmt.Errorf("未知的数据项 %q", kind)
	}
	workbook := excel.New()
	defer workbook.Close()
	if err := workbook.SetSheetName("Sheet1", "数据"); err != nil {
		return err
	}
	if _, err := workbook.NewSheet("说明"); err != nil {
		return err
	}
	headerStyle, err := workbook.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#E8E4DA"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "#B9B1A1", Style: 1},
			{Type: "right", Color: "#B9B1A1", Style: 1},
			{Type: "top", Color: "#B9B1A1", Style: 1},
			{Type: "bottom", Color: "#B9B1A1", Style: 1},
		},
	})
	if err != nil {
		return err
	}
	textStyle, err := workbook.NewStyle(&excelize.Style{
		NumFmt: 49,
		Border: []excelize.Border{
			{Type: "left", Color: "#D8D2C4", Style: 1},
			{Type: "right", Color: "#D8D2C4", Style: 1},
			{Type: "top", Color: "#D8D2C4", Style: 1},
			{Type: "bottom", Color: "#D8D2C4", Style: 1},
		},
	})
	if err != nil {
		return err
	}
	if err := workbook.SetSheetRow("数据", "A1", &headers); err != nil {
		return err
	}
	example := TemplateExample(kind, subjects)
	if err := workbook.SetSheetRow("数据", "A2", &example); err != nil {
		return err
	}
	for index := range headers {
		cell, err := excelize.CoordinatesToCellName(index+1, 1)
		if err != nil {
			return err
		}
		if err := workbook.SetCellStyle("数据", cell, cell, headerStyle); err != nil {
			return err
		}
		body, err := excelize.CoordinatesToCellName(index+1, 2)
		if err != nil {
			return err
		}
		if err := workbook.SetCellStyle("数据", body, body, textStyle); err != nil {
			return err
		}
		width := 14.0
		if len(headers[index]) > 6 {
			width = 20
		}
		column, err := excelize.ColumnNumberToName(index + 1)
		if err != nil {
			return err
		}
		if err := workbook.SetColWidth("数据", column, column, width); err != nil {
			return err
		}
	}
	if err := workbook.SetPanes("数据", &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottom" + "Left"}); err != nil {
		return err
	}
	description := TemplateDescription(kind, subjects)
	for index, row := range description {
		cell, err := excelize.CoordinatesToCellName(1, index+1)
		if err != nil {
			return err
		}
		if err := workbook.SetSheetRow("说明", cell, &row); err != nil {
			return err
		}
		if index == 0 {
			last, err := excelize.CoordinatesToCellName(len(row), index+1)
			if err != nil {
				return err
			}
			if err := workbook.SetCellStyle("说明", cell, last, headerStyle); err != nil {
				return err
			}
		}
	}
	if err := workbook.SetColWidth("说明", "A", "C", 32); err != nil {
		return err
	}
	if err := workbook.SaveAs(path); err != nil {
		return err
	}
	return nil
}
