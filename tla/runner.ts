// The command line's checks run in one worker thread, so the main thread stays
// free while a long search holds the worker: it prints each progress line,
// prints where a stopped check had got to, enforces the memory limit, and
// removes the spill directory on every exit.
//
// The worker is this module. The main thread sends it one model or mutant at a
// time; the worker checks it with checkModel or checkMutant and posts its
// progress and then its result. Only the worker loads check.js, and lazily:
// check.js imports this module, and under Node a static import back, or a
// top-level await on it, never settles.

import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  isMainThread,
  type MessagePort,
  parentPort,
  Worker,
  workerData,
} from 'node:worker_threads'
import type {
  Model,
  ModelResult,
  Mutant,
  MutantResult,
  Progress,
} from './check.js'

// Job is one check the main thread sends the worker.
type Job =
  | { kind: 'model'; model: Model }
  | { kind: 'mutant'; spec: string; mutant: Mutant }

// Reply is what the worker posts back for the job it is running.
type Reply =
  | { kind: 'progress'; progress: Progress }
  | { kind: 'result'; result: ModelResult | MutantResult }

// RunnerData is the worker's workerData.
interface RunnerData {
  shared: string
  workers: number
}

// Runner checks models and mutants in the worker.
export interface Runner {
  checkModel: (model: Model, name: string) => Promise<ModelResult>
  checkMutant: (
    spec: string,
    mutant: Mutant,
    name: string,
  ) => Promise<MutantResult>
  // close stops the worker and removes the spill directory.
  close: () => Promise<void>
}

const MiB = 1024 * 1024

// formatMiB shows bytes in whole MiB.
const formatMiB = (bytes: number) => `${Math.round(bytes / MiB)} MiB`

// formatProgress shows how far a check has got: its counts, the memory its
// seen states and spilled queue take when the checker reports them, and the
// process's RSS. spillBytes, when given, is measured now and replaces the
// checker's figure, which can be up to a minute old.
export function formatProgress(
  kind: string,
  name: string,
  ms: number,
  p: Progress | undefined,
  rss: number,
  spillBytes?: number,
): string {
  const parts = [kind.padEnd(8), name, `${(ms / 1000).toFixed(0)}s`]
  if (p) {
    parts.push(
      `distinct ${p.distinctStates}`,
      `generated ${p.generatedStates}`,
      `depth ${p.depth}`,
      `queue ${p.queuedStates}`,
    )
    if (p.seenBytes !== undefined) parts.push(`seen ${formatMiB(p.seenBytes)}`)
  }
  const spill = spillBytes ?? p?.spillBytes
  if (spill !== undefined) {
    const each =
      p?.spillStates && p.spillBytes !== undefined
        ? ` (${p.spillStates} states, ${Math.round(p.spillBytes / p.spillStates)} B each)`
        : ''
    parts.push(`spill ${formatMiB(spill)}${each}`)
  }
  parts.push(`rss ${formatMiB(rss)}`)
  return parts.join('  ')
}

// directoryBytes returns the bytes of the files under dir, skipping any that
// vanish while it walks.
function directoryBytes(dir: string): number {
  let bytes = 0
  let entries: fs.Dirent[]
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true })
  } catch {
    return 0
  }
  for (const entry of entries) {
    const file = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      bytes += directoryBytes(file)
      continue
    }
    try {
      bytes += fs.statSync(file).size
    } catch {
      // The checker deleted the file as it read it back.
    }
  }
  return bytes
}

// startRunner starts the worker. The checker spills its queue into a new
// directory under TLA_SPILL, or the system's temporary directory, through the
// worker's TMPDIR. With limitBytes set, a check whose RSS plus spill passes it
// is stopped. A stopped check, by the limit, SIGINT or SIGTERM, prints its
// last progress with the RSS and spill of the moment and exits the process.
export function startRunner(
  shared: string,
  workers: number,
  limitBytes = Infinity,
): Runner {
  const spill = fs.mkdtempSync(
    path.join(process.env.TLA_SPILL ?? os.tmpdir(), 'aptre-tla-'),
  )
  const worker = new Worker(fileURLToPath(import.meta.url), {
    workerData: { shared, workers } satisfies RunnerData,
    env: { ...process.env, TMPDIR: spill },
  })
  const removeSpill = () => fs.rmSync(spill, { recursive: true, force: true })

  // The check in flight, and the last progress it posted.
  let current:
    | {
        name: string
        start: number
        progress?: Progress
        resolve: (result: ModelResult | MutantResult) => void
        reject: (err: Error) => void
      }
    | undefined

  const stop = (kind: string, code: number) => {
    const ms = current ? performance.now() - current.start : 0
    console.error(
      formatProgress(
        kind,
        current?.name ?? '-',
        ms,
        current?.progress,
        process.memoryUsage.rss(),
        directoryBytes(spill),
      ),
    )
    void worker.terminate()
    removeSpill()
    process.exit(code)
  }
  const onSigint = () => stop('stopped', 130)
  const onSigterm = () => stop('stopped', 143)
  process.on('SIGINT', onSigint)
  process.on('SIGTERM', onSigterm)
  const guard =
    limitBytes === Infinity
      ? undefined
      : setInterval(() => {
          if (process.memoryUsage.rss() + directoryBytes(spill) > limitBytes)
            stop('killed', 1)
        }, 1000)
  guard?.unref()

  worker.on('message', (reply: Reply) => {
    if (!current) return
    if (reply.kind === 'progress') {
      current.progress = reply.progress
      console.error(
        formatProgress(
          'progress',
          current.name,
          performance.now() - current.start,
          reply.progress,
          process.memoryUsage.rss(),
        ),
      )
      return
    }
    const done = current
    current = undefined
    done.resolve(reply.result)
  })
  // A worker that fails or exits mid-check, such as out of heap, fails that
  // check rather than leaving it pending forever.
  worker.on('error', (err) => {
    const done = current
    current = undefined
    done?.reject(err)
  })
  worker.on('exit', (code) => {
    const done = current
    current = undefined
    done?.reject(new Error(`checker worker exited with code ${code}`))
  })

  const run = <R extends ModelResult | MutantResult>(job: Job, name: string) =>
    new Promise<R>((resolve, reject) => {
      current = {
        name,
        start: performance.now(),
        resolve: resolve as (result: ModelResult | MutantResult) => void,
        reject,
      }
      worker.postMessage(job)
    })
  return {
    checkModel: (model, name) => run({ kind: 'model', model }, name),
    checkMutant: (spec, mutant, name) =>
      run({ kind: 'mutant', spec, mutant }, name),
    close: async () => {
      if (guard) clearInterval(guard)
      process.off('SIGINT', onSigint)
      process.off('SIGTERM', onSigterm)
      await worker.terminate()
      removeSpill()
    },
  }
}

if (!isMainThread && parentPort) {
  void serveJobs(parentPort)
}

// serveJobs runs the worker: it checks each job the main thread sends, posting
// its progress and then its result. It imports check.js without awaiting it at
// the top level, so this module finishes evaluating before check.js, which
// imports it, needs it; the port queues any job sent in the meantime.
async function serveJobs(port: MessagePort): Promise<void> {
  const { checkModel, checkMutant } = await import('./check.js')
  const { shared, workers } = workerData as RunnerData
  const onProgress = (progress: Progress) =>
    port.postMessage({ kind: 'progress', progress } satisfies Reply)
  port.on('message', async (job: Job) => {
    const result =
      job.kind === 'model'
        ? await checkModel(job.model, shared, workers, onProgress)
        : await checkMutant(job.spec, job.mutant, shared, workers, onProgress)
    port.postMessage({ kind: 'result', result } satisfies Reply)
  })
}
