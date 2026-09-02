/**
 * Typecheck ratchet: vue-tsc has a large pre-existing error backlog, so a
 * plain `pnpm typecheck` cannot gate CI. This runs it, counts errors per
 * file and fails only when a file gained errors or a new file started
 * failing, compared with typecheck-baseline.json. Fixing errors lowers the
 * baseline automatically on the next `--update`.
 *
 *   node scripts/typecheck-gate.mjs            # gate against the baseline
 *   node scripts/typecheck-gate.mjs --update   # rewrite the baseline
 */
import {spawnSync} from 'node:child_process'
import {readFileSync, writeFileSync, existsSync} from 'node:fs'
import {resolve} from 'node:path'

const BASELINE = resolve('typecheck-baseline.json')
const update = process.argv.includes('--update')

const run = spawnSync('pnpm', ['exec', 'vue-tsc', '--build', '--force'], {encoding: 'utf8', maxBuffer: 64 * 1024 * 1024})
const output = (run.stdout || '') + (run.stderr || '')

// vue-tsc prints `path(line,col): error TSxxxx: message`; count per path.
const counts = {}
for (const line of output.split('\n')) {
	const m = line.match(/^(.+?)\(\d+,\d+\): error TS\d+/)
	if (m) {
		const file = m[1].replace(/\\/g, '/')
		counts[file] = (counts[file] || 0) + 1
	}
}
const total = Object.values(counts).reduce((a, b) => a + b, 0)

if (update) {
	const sorted = Object.fromEntries(Object.entries(counts).sort(([a], [b]) => a.localeCompare(b)))
	writeFileSync(BASELINE, JSON.stringify({total, files: sorted}, null, '\t') + '\n')
	console.log(`typecheck baseline written: ${total} error(s) in ${Object.keys(counts).length} file(s)`)
	process.exit(0)
}

if (!existsSync(BASELINE)) {
	console.error(`no ${BASELINE}; run with --update first`)
	process.exit(2)
}
const baseline = JSON.parse(readFileSync(BASELINE, 'utf8'))
const regressions = []
for (const [file, n] of Object.entries(counts)) {
	const before = baseline.files[file] || 0
	if (n > before) {
		regressions.push(`${file}: ${before} -> ${n}`)
	}
}

console.log(`typecheck: ${total} error(s) now, ${baseline.total} in the baseline`)
if (regressions.length > 0) {
	console.error('\nTypecheck regressions (errors per file went up or a clean file broke):')
	for (const r of regressions) {
		console.error(`  ${r}`)
	}
	console.error('\nFix them, or if the baseline is genuinely stale run: pnpm typecheck:baseline')
	// Print the offending lines so CI logs are actionable.
	const bad = new Set(regressions.map(r => r.split(':')[0]))
	for (const line of output.split('\n')) {
		const m = line.match(/^(.+?)\(\d+,\d+\): error TS\d+/)
		if (m && bad.has(m[1].replace(/\\/g, '/'))) {
			console.error(line)
		}
	}
	process.exit(1)
}
if (total < baseline.total) {
	console.log(`errors went down by ${baseline.total - total}; run pnpm typecheck:baseline to lock that in`)
}
