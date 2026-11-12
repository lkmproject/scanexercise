import { File } from 'expo-file-system';
import { EncodingType, writeAsStringAsync } from 'expo-file-system/legacy';

import { API_BASE_URL } from './config';
import { authHeaders, notifyUnauthorized } from './session';

export class UnauthorizedError extends Error {
  constructor() {
    super('unauthorized');
    this.name = 'UnauthorizedError';
  }
}

export function isUnauthorized(error: unknown): boolean {
  return error instanceof UnauthorizedError || (error instanceof Error && error.name === 'UnauthorizedError');
}

export const ApiCode = {
  OK: 200,
  AuthInvalid: 1001,
  AuthUnavailable: 1002,
  AuthFailed: 1003,
  Unauthorized: 1004,
  InvalidRequest: 1101,
  NoPages: 1102,
  InvalidImage: 1103,
  TooManyPages: 1104,
  ImageTooLarge: 1105,
  JobNotFound: 1201,
  JobNotReady: 1202,
  JobFailed: 1203,
  PageNotFound: 1204,
  PDFFailed: 1301,
  InsufficientCredits: 1401,
  Internal: 1901,
} as const;

export type ApiCode = (typeof ApiCode)[keyof typeof ApiCode];

export type JobStatus = 'pending' | 'running' | 'succeeded' | 'failed';

export type Account = {
  user_id: string;
  email: string;
  name: string;
  remaining: number;
  token: string;
};

export type Job = {
  id: string;
  status: JobStatus;
  page_count: number;
  error: string | null;
  remaining?: number;
  previews?: string[];
  pdf?: string;
};

type Envelope<T> = {
  code: number;
  reason: string;
  data: T | null;
};

export async function getMe(): Promise<Account> {
  const response = await fetch(`${API_BASE_URL}/me`, {
    headers: authHeaders(),
  });
  return readJson<Account>(response, 'me');
}

export async function loginGoogle(idToken: string): Promise<Account> {
  const response = await fetch(`${API_BASE_URL}/auth/google`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id_token: idToken }),
  });
  return readJson<Account>(response, 'login');
}

export async function createJob(
  uris: string[],
  crop = false,
  binarization = false,
): Promise<{ id: string; status: JobStatus; remaining: number }> {
  const files = await requestUploadSlots(uris.length);
  const keys = await Promise.all(
    uris.map(async (uri, i) => {
      const slot = files[i];
      if (slot == null || slot.url === '' || slot.key === '') {
        throw new Error('slots mismatch');
      }
      await putJpeg(slot.url, uri);
      return slot.key;
    }),
  );
  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}/jobs`, {
      method: 'POST',
      headers: { ...authHeaders(), 'Content-Type': 'application/json' },
      body: JSON.stringify({ keys, crop: crop ? 1 : 0, binarization: binarization ? 1 : 0 }),
    });
  } catch (caught) {
    throw new Error(networkFail('jobs', caught));
  }
  return readJson(response, 'jobs');
}

async function requestUploadSlots(count: number): Promise<{ key: string; url: string }[]> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}/uploads`, {
      method: 'POST',
      headers: { ...authHeaders(), 'Content-Type': 'application/json' },
      body: JSON.stringify({ count }),
    });
  } catch (caught) {
    throw new Error(networkFail('slots', caught));
  }
  const data = await readJson<{ files: { key: string; url: string }[] }>(response, 'slots');
  if (!Array.isArray(data.files) || data.files.length !== count) {
    throw new Error('slots mismatch');
  }
  return data.files;
}

async function putJpeg(url: string, uri: string): Promise<void> {
  await new Promise<void>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve();
        return;
      }
      const code = /<Code>([^<]+)<\/Code>/.exec(xhr.responseText ?? '')?.[1];
      reject(new Error(`upload failed ${xhr.status}${code ? ` ${code}` : ''}`));
    };
    xhr.onerror = () => reject(new Error(`upload network ${xhr.status}`));
    xhr.ontimeout = () => reject(new Error('upload timeout'));
    xhr.open('PUT', url);
    xhr.setRequestHeader('Content-Type', 'image/jpeg');
    xhr.timeout = 120000;
    xhr.send({ uri, type: 'image/jpeg', name: 'page.jpg' } as unknown as XMLHttpRequestBodyInit);
  });
}

export async function getJob(id: string): Promise<Job> {
  const response = await fetch(`${API_BASE_URL}/jobs/${id}`, {
    headers: authHeaders(),
  });
  return readJson(response, 'job');
}

export async function createPdf(uris: string[], destination: string): Promise<void> {
  const body = new FormData();
  uris.forEach((uri, index) => {
    body.append('pages', new File(uri), `page-${index}.jpg`);
  });
  const response = await fetch(`${API_BASE_URL}/pdf`, {
    method: 'POST',
    headers: authHeaders(),
    body,
  });
  const contentType = response.headers.get('content-type') ?? '';
  if (contentType.includes('application/json')) {
    await readJson(response, 'pdf');
    throw new Error('pdf failed');
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

export function failFromEnvelope(payload: { code?: number; reason?: string; message?: string }, step = 'api'): never {
  if (payload.code === ApiCode.Unauthorized) {
    throwUnauthorized();
  }
  const detail = payload.reason || payload.message || JSON.stringify(payload);
  throw new Error(`${step}: ${detail}`);
}

async function readJson<T>(response: Response, step = 'api'): Promise<T> {
  const text = await response.text();
  let payload: Envelope<T> & { message?: string };
  try {
    payload = JSON.parse(text) as Envelope<T> & { message?: string };
  } catch {
    throw new Error(formatHttpFail(response.status, text, `${step} bad response`));
  }
  if (payload.code !== ApiCode.OK || payload.data == null) {
    failFromEnvelope(payload, step);
  }
  return payload.data;
}

async function httpFail(response: Response, fallback: string): Promise<string> {
  let text = '';
  try {
    text = await response.text();
  } catch {
    text = '';
  }
  return formatHttpFail(response.status, text, fallback);
}

function formatHttpFail(status: number, text: string, fallback: string): string {
  const detail = /<Code>([^<]+)<\/Code>/.exec(text)?.[1] ?? text.slice(0, 80);
  const suffix = detail.trim() ? ` ${detail.trim()}` : '';
  return `${fallback} ${status}${suffix}`;
}

function networkFail(step: string, caught: unknown): string {
  const message = caught instanceof Error ? caught.message : '';
  return message !== '' ? `${step} network ${message}` : `${step} network failed`;
}

function throwUnauthorized(): never {
  notifyUnauthorized();
  throw new UnauthorizedError();
}
