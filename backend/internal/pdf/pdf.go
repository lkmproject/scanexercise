package pdf

import (
	"fmt"
	"strings"
)

const (
	a4Short = 595.28
	a4Long  = 841.89
)

func Build(pages [][]byte) ([]byte, error) {
	if len(pages) == 0 {
		return nil, fmt.Errorf("沒有頁面")
	}
	sizes := make([][2]int, len(pages))
	for i, page := range pages {
		width, height, err := jpegSize(page)
		if err != nil {
			return nil, err
		}
		sizes[i] = [2]int{width, height}
	}

	body := []byte("%PDF-1.4\n")
	offsets := []int{0}

	write := func(number int, content []byte) {
		offsets = append(offsets, len(body))
		body = append(body, []byte(fmt.Sprintf("%d 0 obj\n", number))...)
		body = append(body, content...)
		body = append(body, []byte("\nendobj\n")...)
	}

	kids := make([]string, len(pages))
	for index := range pages {
		kids[index] = fmt.Sprintf("%d 0 R", 3+index*3)
	}
	write(1, []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	write(2, []byte(fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", len(pages), strings.Join(kids, " "))))

	for index, jpeg := range pages {
		pageNumber := 3 + index*3
		contentNumber := pageNumber + 1
		imageNumber := pageNumber + 2
		pageWidth, pageHeight, x, y, drawWidth, drawHeight := fitA4(sizes[index][0], sizes[index][1])
		write(pageNumber, []byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Contents %d 0 R /Resources << /XObject << /Im0 %d 0 R >> >> >>",
			pageWidth, pageHeight, contentNumber, imageNumber,
		)))
		stream := []byte(fmt.Sprintf("q %.2f 0 0 %.2f %.2f %.2f cm /Im0 Do Q", drawWidth, drawHeight, x, y))
		content := append([]byte(fmt.Sprintf("<< /Length %d >>\nstream\n", len(stream))), stream...)
		content = append(content, []byte("\nendstream")...)
		write(contentNumber, content)
		imageObj := append([]byte(fmt.Sprintf(
			"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n",
			sizes[index][0], sizes[index][1], len(jpeg),
		)), jpeg...)
		imageObj = append(imageObj, []byte("\nendstream")...)
		write(imageNumber, imageObj)
	}

	startxref := len(body)
	body = append(body, []byte(fmt.Sprintf("xref\n0 %d\n", len(offsets)))...)
	body = append(body, []byte("0000000000 65535 f \n")...)
	for _, offset := range offsets[1:] {
		body = append(body, []byte(fmt.Sprintf("%010d 00000 n \n", offset))...)
	}
	body = append(body, []byte(fmt.Sprintf(
		"trailer << /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(offsets), startxref,
	))...)
	return body, nil
}

func fitA4(width, height int) (pageWidth, pageHeight, x, y, drawWidth, drawHeight float64) {
	if height >= width {
		pageWidth, pageHeight = a4Short, a4Long
	} else {
		pageWidth, pageHeight = a4Long, a4Short
	}
	scale := min(pageWidth/float64(width), pageHeight/float64(height))
	drawWidth = float64(width) * scale
	drawHeight = float64(height) * scale
	x = (pageWidth - drawWidth) / 2
	y = (pageHeight - drawHeight) / 2
	return
}

func jpegSize(data []byte) (int, int, error) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, 0, fmt.Errorf("結果不是 JPEG")
	}
	index := 2
	for index+1 < len(data) {
		if data[index] != 0xFF {
			return 0, 0, fmt.Errorf("結果不是 JPEG")
		}
		for index < len(data) && data[index] == 0xFF {
			index++
		}
		if index >= len(data) {
			break
		}
		marker := data[index]
		index++
		if marker == 0xD8 || marker == 0xD9 {
			continue
		}
		if marker == 0xDA || index+2 > len(data) {
			break
		}
		length := int(data[index])<<8 | int(data[index+1])
		if (marker == 0xC0 || marker == 0xC1 || marker == 0xC2) && index+7 <= len(data) {
			height := int(data[index+3])<<8 | int(data[index+4])
			width := int(data[index+5])<<8 | int(data[index+6])
			return width, height, nil
		}
		index += length
	}
	return 0, 0, fmt.Errorf("結果不是 JPEG")
}
