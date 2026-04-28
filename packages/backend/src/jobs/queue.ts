import PgBoss from 'pg-boss';
import { env } from '../utils/env.js';
import { registerJobHandlers } from './handlers.js';

let boss: PgBoss | null = null;

export async function createJobQueue(): Promise<PgBoss> {
  boss = new PgBoss(env.DATABASE_URL);

  boss.on('error', (error) => {
    console.error('pg-boss error:', error);
  });

  await boss.start();
  await registerJobHandlers(boss);

  return boss;
}

export function getJobQueue(): PgBoss {
  if (!boss) throw new Error('Job queue not initialized');
  return boss;
}
