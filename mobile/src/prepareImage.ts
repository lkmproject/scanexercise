import { Image } from 'react-native';
import { ImageManipulator, SaveFormat } from 'expo-image-manipulator';

import { MAX_LONG_EDGE } from './config';

export async function fitForUpload(uri: string): Promise<string> {
  const size = await imageSize(uri);
  const jpeg = /\.jpe?g($|\?)/i.test(uri);
  if (jpeg && size != null && Math.max(size.width, size.height) <= MAX_LONG_EDGE) {
    return uri;
  }
  const context = ImageManipulator.manipulate(uri, {
    maxWidth: MAX_LONG_EDGE,
    maxHeight: MAX_LONG_EDGE,
  });
  try {
    const image = await context.renderAsync();
    try {
      const saved = await image.saveAsync({
        compress: 0.97,
        format: SaveFormat.JPEG,
      });
      return saved.uri;
    } finally {
      image.release();
    }
  } finally {
    context.release();
  }
}

function imageSize(uri: string): Promise<{ width: number; height: number } | null> {
  return new Promise((resolve) => {
    Image.getSize(
      uri,
      (width, height) => resolve({ width, height }),
      () => resolve(null),
    );
  });
}
