import { File } from 'expo-file-system';

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

export async function createJob(uris: string[]): Promise<{ id: string; status: JobStatus }> {
  const body = new FormData();
  uris.forEach((uri, index) => {
    body.append('pages', new File(uri), `page-${index}.jpg`);
  });
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

async function readJson(response: Response): Promise<Job> {
  const payload = (await response.json()) as Job & { detail?: string };
  if (!response.ok) {
    throw new Error(payload.detail || '請求失敗');
  }
  return payload;
}
