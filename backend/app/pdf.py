A4_SHORT = 595.28
A4_LONG = 841.89


def build_pdf(pages: list[bytes]) -> bytes:
    if not pages:
        raise ValueError("沒有頁面")
    sizes = [_jpeg_size(page) for page in pages]
    count = len(pages)
    body = bytearray(b"%PDF-1.4\n")
    offsets = [0]

    def write(number: int, content: bytes) -> None:
        offsets.append(len(body))
        body.extend(f"{number} 0 obj\n".encode("ascii"))
        body.extend(content)
        body.extend(b"\nendobj\n")

    kids = " ".join(f"{3 + index * 3} 0 R" for index in range(count))
    write(1, b"<< /Type /Catalog /Pages 2 0 R >>")
    write(2, f"<< /Type /Pages /Count {count} /Kids [{kids}] >>".encode("ascii"))
    for index, (jpeg, size) in enumerate(zip(pages, sizes)):
        page_number = 3 + index * 3
        content_number = page_number + 1
        image_number = page_number + 2
        page_width, page_height, x, y, draw_width, draw_height = _fit_a4(*size)
        write(
            page_number,
            (
                f"<< /Type /Page /Parent 2 0 R "
                f"/MediaBox [0 0 {page_width:.2f} {page_height:.2f}] "
                f"/Contents {content_number} 0 R "
                f"/Resources << /XObject << /Im0 {image_number} 0 R >> >> >>"
            ).encode("ascii"),
        )
        stream = (
            f"q {draw_width:.2f} 0 0 {draw_height:.2f} {x:.2f} {y:.2f} cm /Im0 Do Q"
        ).encode("ascii")
        write(
            content_number,
            f"<< /Length {len(stream)} >>\nstream\n".encode("ascii") + stream + b"\nendstream",
        )
        write(
            image_number,
            (
                f"<< /Type /XObject /Subtype /Image /Width {size[0]} /Height {size[1]} "
                f"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode "
                f"/Length {len(jpeg)} >>\nstream\n"
            ).encode("ascii")
            + jpeg
            + b"\nendstream",
        )

    startxref = len(body)
    body.extend(f"xref\n0 {len(offsets)}\n".encode("ascii"))
    body.extend(b"0000000000 65535 f \n")
    for offset in offsets[1:]:
        body.extend(f"{offset:010d} 00000 n \n".encode("ascii"))
    body.extend(
        (
            f"trailer << /Size {len(offsets)} /Root 1 0 R >>\n"
            f"startxref\n{startxref}\n%%EOF\n"
        ).encode("ascii")
    )
    return bytes(body)


def _fit_a4(width: int, height: int) -> tuple[float, float, float, float, float, float]:
    portrait = height >= width
    page_width, page_height = (A4_SHORT, A4_LONG) if portrait else (A4_LONG, A4_SHORT)
    scale = min(page_width / width, page_height / height)
    draw_width = width * scale
    draw_height = height * scale
    x = (page_width - draw_width) / 2
    y = (page_height - draw_height) / 2
    return page_width, page_height, x, y, draw_width, draw_height


def _jpeg_size(data: bytes) -> tuple[int, int]:
    if len(data) < 4 or data[:2] != b"\xff\xd8":
        raise ValueError("結果不是 JPEG")
    index = 2
    while index + 1 < len(data):
        if data[index] != 0xFF:
            raise ValueError("結果不是 JPEG")
        while index < len(data) and data[index] == 0xFF:
            index += 1
        if index >= len(data):
            break
        marker = data[index]
        index += 1
        if marker in {0xD8, 0xD9}:
            continue
        if marker == 0xDA or index + 2 > len(data):
            break
        length = int.from_bytes(data[index : index + 2], "big")
        if marker in {0xC0, 0xC1, 0xC2} and index + 7 <= len(data):
            height = int.from_bytes(data[index + 3 : index + 5], "big")
            width = int.from_bytes(data[index + 5 : index + 7], "big")
            return width, height
        index += length
    raise ValueError("結果不是 JPEG")
