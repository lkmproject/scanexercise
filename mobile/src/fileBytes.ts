import { File } from 'expo-file-system';
import { getInfoAsync } from 'expo-file-system/legacy';

export async function uriBytes(uri: string): Promise<number> {
  try {
    const size = new File(uri).size;
    if (size > 0) {
      return size;
    }
  } catch {
    // fall through
  }
  try {
    const info = await getInfoAsync(uri);
    if (info.exists && 'size' in info && typeof info.size === 'number') {
      return info.size;
    }
  } catch {
    return 0;
  }
  return 0;
}
