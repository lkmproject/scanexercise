import { File } from 'expo-file-system';
import { EncodingType, writeAsStringAsync } from 'expo-file-system/legacy';

import { API_BASE_URL } from './config';

export type JobStatus = 'pending' | 'running' | 'succeeded' | 'failed';

export type Job = {
  id: string;
  status: JobStatus;
  page_count: number;
  error: string | null;
  previews?: string[];
  pdf?: string;
};

export async function createJob(
  uris: string[],
  crop = false,
): Promise<{ id: string; status: JobStatus }> {
  const body = new FormData();
  uris.forEach((uri, index) => {
    body.append('pages', new File(uri), `page-${index}.jpg`);
  });
  body.append('crop', crop ? '1' : '0');
  const response = await fetch(`${API_BASE_URL}/jobs`, {
    method: 'POST',
    body,
  });
  return readJson(response);
}

export async function getJob(id: string): Promise<Job> {
  const response = await fetch(`${API_BASE_URL}/jobs/${id}`);
  return readJson(response);
}

export async function createPdf(uris: string[], destination: string): Promise<void> {
  const body = new FormData();
  uris.forEach((uri, index) => {
    body.append('pages', new File(uri), `page-${index}.jpg`);
  });
  const response = await fetch(`${API_BASE_URL}/pdf`, {
    method: 'POST',
    body,
  });
  if (!response.ok) {
    const payload = (await response.json()) as { detail?: string };
    throw new Error(payload.detail || '請求失敗');
  }
  const bytes = new Uint8Array(await response.arrayBuffer());
  await writeAsStringAsync(destination, bytesToBase64(bytes), {
    encoding: EncodingType.Base64,
  });
}

function bytesToBase64(bytes: Uint8Array): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
  let output = '';
  for (let i = 0; i < bytes.length; i += 3) {
    const a = bytes[i] ?? 0;
    const b = bytes[i + 1] ?? 0;
    const c = bytes[i + 2] ?? 0;
    const triple = (a << 16) | (b << 8) | c;
    output += chars[(triple >> 18) & 63];
    output += chars[(triple >> 12) & 63];
    output += i + 1 < bytes.length ? chars[(triple >> 6) & 63] : '=';
    output += i + 2 < bytes.length ? chars[triple & 63] : '=';
  }
  return output;
}

async function readJson(response: Response): Promise<Job> {
  const payload = (await response.json()) as Job & { detail?: string };
  if (!response.ok) {
    throw new Error(payload.detail || '請求失敗');
  }
  return payload;
}
