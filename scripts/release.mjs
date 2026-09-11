import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, writeFileSync, mkdirSync, cpSync, readdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve, join } from 'node:path';

const repo = 'azohra/gopro-yank';
const components = { app: 'v', site: 'site/v' };
const run = (command, args, options = {}) => (execFileSync(command, args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'inherit'], ...options }) ?? '').trim();
const git = (...args) => run('git', args);
const gh = (...args) => run('gh', args);
const api = path => JSON.parse(gh('api', `repos/${repo}/${path}`));
const read = path => readFileSync(path, 'utf8');
const mise = (task, args = []) => run('mise', ['run', task, '--', ...args]);
const requireThat = (condition, message) => { if (!condition) throw new Error(message); };
const version = component => read(`${component}/VERSION`).trim();
const tagFor = component => components[component] + version(component);
const changed = (component, base) => git('diff', base, 'HEAD', '--', `${component}/VERSION`) !== '';
const releaseFiles = Object.keys(components).flatMap(component => [`${component}/VERSION`, `${component}/CHANGELOG.md`]);

function prepare() {
  requireThat(git('status', '--porcelain') === '', 'Commit working changes before preparing a release.');
  git('fetch', 'origin', 'main', '--tags');
  requireThat(git('rev-parse', 'HEAD') === git('rev-parse', 'origin/main'), 'Prepare from current main.');
  mkdirSync('release', { recursive: true });
  for (const [component, prefix] of Object.entries(components)) {
    const context = mise(`//${component}:changelog`, ['--unreleased', '--bump', '--context']);
    const candidate = JSON.parse(context)[0];
    if (!candidate?.commits.length || candidate.version === tagFor(component)) continue;
    requireThat(new RegExp(`^${prefix}[0-9]+\\.[0-9]+\\.[0-9]+$`).test(candidate.version), 'Expected a stable component version.');
    const contextFile = resolve('release', `${component}.json`);
    writeFileSync(contextFile, context);
    const notes = mise(`//${component}:changelog`, ['--from-context', contextFile, '--strip', 'all']);
    writeFileSync(`${component}/VERSION`, candidate.version.slice(prefix.length) + '\n');
    const previous = read(`${component}/CHANGELOG.md`).replace(/^# Changelog\s*/, '');
    writeFileSync(`${component}/CHANGELOG.md`, `# Changelog\n\n${notes}\n\n${previous}`);
  }
}

function build() {
  requireThat(git('status', '--porcelain') === '', 'Commit working changes before building a release.');
  git('fetch', 'origin', 'main', '--tags');
  const base = git('rev-parse', 'origin/main');
  requireThat(git('rev-parse', 'HEAD^') === base, 'Refresh the release PR from current main before building.');
  const files = git('diff', '--name-only', base, 'HEAD').split('\n');
  requireThat(files.every(file => releaseFiles.includes(file)), 'A release PR may only change component versions and changelogs.');
  rmSync('release', { recursive: true, force: true });
  mkdirSync('release');
  for (const component of Object.keys(components).filter(component => changed(component, base))) {
    const expected = mise(`//${component}:changelog`, ['--offline', '--unreleased', '--bumped-version', '--skip-commit', git('rev-parse', 'HEAD')]);
    requireThat(expected === tagFor(component), `Refresh the proposed ${component} version with git-cliff.`);
    run('mise', ['run', `//${component}:check`, ':::', `//${component}:build:dist`], { stdio: 'inherit', env: { ...process.env, RELEASE_VERSION: version(component) } });
    const destination = `release/${component}`;
    cpSync(`${component}/release`, destination, { recursive: true });
    const previous = git('show', `${base}:${component}/CHANGELOG.md`).replace(/^# Changelog\s*/, '');
    const changelog = read(`${component}/CHANGELOG.md`).replace(/^# Changelog\s*/, '').trimEnd();
    requireThat(changelog.endsWith(previous), 'Preserve the published changelog when preparing a release.');
    writeFileSync(`${destination}/notes.md`, (previous ? changelog.slice(0, -previous.length) : changelog).trim() + '\n');
  }
  requireThat(readdirSync('release').length > 0, 'The release PR contains no version changes.');
}

function publish() {
  const sha = git('rev-parse', 'HEAD');
  const pr = api(`commits/${sha}/pulls`).find(pr => pr.merged_at && pr.merge_commit_sha === sha && pr.head.ref === 'release/next' && pr.head.repo?.full_name === repo && pr.base.ref === 'main');
  if (!pr) return;
  const name = `release-${git('rev-parse', 'HEAD^{tree}')}`;
  const artifacts = JSON.parse(gh('api', '--paginate', '--slurp', `repos/${repo}/actions/artifacts?name=${name}`)).flatMap(page => page.artifacts);
  let artifact;
  for (const candidate of artifacts) {
    if (candidate.expired || candidate.workflow_run.head_sha !== pr.head.sha) continue;
    const workflow = api(`actions/runs/${candidate.workflow_run.id}`);
    if (workflow.conclusion === 'success' && workflow.path === '.github/workflows/check.yml' && ['pull_request', 'workflow_dispatch'].includes(workflow.event)) {
      artifact = candidate;
      break;
    }
  }
  requireThat(artifact, 'No successful Check artifact matches this merged release; publication has stopped.');
  const scratch = mkdtempSync(join(tmpdir(), 'gopro-release-'));
  try {
    gh('run', 'download', String(artifact.workflow_run.id), '--repo', repo, '--name', name, '--dir', scratch);
    for (const component of Object.keys(components).filter(component => changed(component, 'HEAD^'))) {
      const tag = tagFor(component);
      const remoteTag = git('ls-remote', 'origin', `refs/tags/${tag}`, `refs/tags/${tag}^{}`);
      if (remoteTag) {
        const refs = remoteTag.split('\n');
        const target = refs.find(ref => ref.endsWith('^{}')) ?? refs[0];
        requireThat(target.split('\t')[0] === sha, `${tag} already belongs to another commit.`);
      }
      const directory = join(scratch, component);
      const files = readdirSync(directory).filter(file => file !== 'notes.md').map(file => join(directory, file));
      requireThat(files.length > 0, `The ${component} release artifact is empty.`);
      const releases = JSON.parse(gh('api', '--paginate', '--slurp', `repos/${repo}/releases`)).flat();
      const existing = releases.find(release => release.tag_name === tag);
      if (existing && !existing.draft) {
        git('fetch', 'origin', `refs/tags/${tag}:refs/tags/${tag}`);
        requireThat(git('rev-parse', `${tag}^{commit}`) === sha, `${tag} already belongs to another commit.`);
      } else {
        if (existing) requireThat(existing.target_commitish === sha, 'The existing draft targets another commit.');
        else gh('release', 'create', tag, '--repo', repo, '--draft', '--target', sha, '--title', `${component === 'app' ? 'GoPro Yank' : 'Website'} ${tag}`, '--notes-file', join(directory, 'notes.md'));
        gh('release', 'upload', tag, ...files, '--repo', repo, '--clobber');
        gh('release', 'edit', tag, '--repo', repo, '--draft=false', `--latest=${component === 'app'}`);
      }
      if (component === 'site') console.log(`site_tag=${tag}`);
    }
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
}

try {
  const command = { prepare, build, publish }[process.argv[2]];
  requireThat(command, 'Use prepare, build or publish.');
  command();
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}
