import { File } from 'expo-file-system';

const a4Short = 595.28;
const a4Long = 841.89;

export async function writeExamPdf(uris: string[], destination: string): Promise<void> {
  const pages: Uint8Array[] = [];
  for (const uri of uris) {
    pages.push(new Uint8Array(await new File(uri).bytes()));
  }
  const pdf = buildPdf(pages);
  const file = new File(destination);
  if (file.exists) {
    file.delete();
  }
  file.create();
  await file.write(pdf);
}

export function buildPdf(pages: Uint8Array[]): Uint8Array {
  if (pages.length === 0) {
    throw new Error('pdf: cannot create pdf');
  }
  const sizes = pages.map((page) => jpegSize(page));
  const body = new Buf();
  body.text('%PDF-1.4\n');
  const offsets = [0];

  const write = (number: number, content: Uint8Array) => {
    offsets.push(body.length);
    body.text(`${number} 0 obj\n`);
    body.bytes(content);
    body.text('\nendobj\n');
  };

  const kids = pages.map((_, index) => `${3 + index * 3} 0 R`).join(' ');
  write(1, ascii('<< /Type /Catalog /Pages 2 0 R >>'));
  write(2, ascii(`<< /Type /Pages /Count ${pages.length} /Kids [${kids}] >>`));

  pages.forEach((jpeg, index) => {
    const pageNumber = 3 + index * 3;
    const contentNumber = pageNumber + 1;
    const imageNumber = pageNumber + 2;
    const fitted = fitA4(sizes[index][0], sizes[index][1]);
    write(
      pageNumber,
      ascii(
        `<< /Type /Page /Parent 2 0 R /MediaBox [0 0 ${n(fitted.pageWidth)} ${n(fitted.pageHeight)}] /Contents ${contentNumber} 0 R /Resources << /XObject << /Im0 ${imageNumber} 0 R >> >> >>`,
      ),
    );
    const stream = ascii(
      `q ${n(fitted.drawWidth)} 0 0 ${n(fitted.drawHeight)} ${n(fitted.x)} ${n(fitted.y)} cm /Im0 Do Q`,
    );
    const content = new Buf();
    content.text(`<< /Length ${stream.length} >>\nstream\n`);
    content.bytes(stream);
    content.text('\nendstream');
    write(contentNumber, content.toUint8Array());
    const image = new Buf();
    image.text(
      `<< /Type /XObject /Subtype /Image /Width ${sizes[index][0]} /Height ${sizes[index][1]} /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length ${jpeg.length} >>\nstream\n`,
    );
    image.bytes(jpeg);
    image.text('\nendstream');
    write(imageNumber, image.toUint8Array());
  });

  const startxref = body.length;
  body.text(`xref\n0 ${offsets.length}\n`);
  body.text('0000000000 65535 f \n');
  for (const offset of offsets.slice(1)) {
    body.text(`${String(offset).padStart(10, '0')} 00000 n \n`);
  }
  body.text(`trailer << /Size ${offsets.length} /Root 1 0 R >>\nstartxref\n${startxref}\n%%EOF\n`);
  return body.toUint8Array();
}

function fitA4(width: number, height: number) {
  const portrait = height >= width;
  const pageWidth = portrait ? a4Short : a4Long;
  const pageHeight = portrait ? a4Long : a4Short;
  const scale = Math.min(pageWidth / width, pageHeight / height);
  const drawWidth = width * scale;
  const drawHeight = height * scale;
  return {
    pageWidth,
    pageHeight,
    x: (pageWidth - drawWidth) / 2,
    y: (pageHeight - drawHeight) / 2,
    drawWidth,
    drawHeight,
  };
}

function jpegSize(data: Uint8Array): [number, number] {
  if (data.length < 4 || data[0] !== 0xff || data[1] !== 0xd8) {
    throw new Error('pdf: cannot create pdf');
  }
  let index = 2;
  while (index + 1 < data.length) {
    if (data[index] !== 0xff) {
      throw new Error('pdf: cannot create pdf');
    }
    while (index < data.length && data[index] === 0xff) {
      index += 1;
    }
    if (index >= data.length) {
      break;
    }
    const marker = data[index];
    index += 1;
    if (marker === 0xd8 || marker === 0xd9) {
      continue;
    }
    if (marker === 0xda || index + 2 > data.length) {
      break;
    }
    const length = (data[index] << 8) | data[index + 1];
    if ((marker === 0xc0 || marker === 0xc1 || marker === 0xc2) && index + 7 <= data.length) {
      const height = (data[index + 3] << 8) | data[index + 4];
      const width = (data[index + 5] << 8) | data[index + 6];
      return [width, height];
    }
    index += length;
  }
  throw new Error('pdf: cannot create pdf');
}

function n(value: number): string {
  return value.toFixed(2);
}

function ascii(value: string): Uint8Array {
  return new TextEncoder().encode(value);
}

class Buf {
  private readonly chunks: Uint8Array[] = [];
  length = 0;

  text(value: string) {
    this.bytes(ascii(value));
  }

  bytes(value: Uint8Array) {
    this.chunks.push(value);
    this.length += value.length;
  }

  toUint8Array(): Uint8Array {
    const out = new Uint8Array(this.length);
    let offset = 0;
    for (const chunk of this.chunks) {
      out.set(chunk, offset);
      offset += chunk.length;
    }
    return out;
  }
}
