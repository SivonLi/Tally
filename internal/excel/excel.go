// Package excel opens and creates the xlsx workbooks Tally imports and exports.
package excel

import "github.com/xuri/excelize/v2"

// Open opens an existing xlsx workbook.
func Open(path string) (*excelize.File, error) {
	return excelize.OpenFile(path)
}

// New returns a new workbook with one worksheet.
func New() *excelize.File {
	return excelize.NewFile()
}
