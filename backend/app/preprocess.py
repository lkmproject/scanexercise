import io

from PIL import Image, ImageOps

try:
    import pillow_heif

    pillow_heif.register_heif_opener()
except ImportError:
    pass


class PreprocessError(Exception):
    pass


def prepare_page(data: bytes, max_long_edge: int) -> bytes:
    if not data:
        raise PreprocessError("無法讀取圖片")
    try:
        with Image.open(io.BytesIO(data)) as image:
            image = ImageOps.exif_transpose(image)
            image = image.convert("RGB")
            image.thumbnail((max_long_edge, max_long_edge), Image.Resampling.LANCZOS)
            return _encode_jpeg(image)
    except PreprocessError:
        raise
    except Exception as exc:
        raise PreprocessError("無法讀取圖片") from exc


def _encode_jpeg(image: Image.Image) -> bytes:
    quality = 90
    encoded = _save_jpeg(image, quality)
    while len(encoded) > 4_000_000 and quality > 50:
        quality -= 10
        encoded = _save_jpeg(image, quality)
    return encoded


def _save_jpeg(image: Image.Image, quality: int) -> bytes:
    buffer = io.BytesIO()
    image.save(buffer, format="JPEG", quality=quality, optimize=True)
    return buffer.getvalue()
