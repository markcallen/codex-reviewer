import React from "react";
import { createRoot } from "react-dom/client";
import {
  ArrowRight,
  CheckCircle2,
  ExternalLink,
  GitBranch,
  Github,
  Globe2,
  ShieldCheck,
  Terminal,
} from "lucide-react";
import "./style.css";

const apiUrl = window.location.origin;
const repoUrl = "https://github.com/markcallen/codex-reviewer";

const commands = [
  {
    label: "Submit through this service",
    value: `codex-reviewer review submit \\
  --api-url ${apiUrl} \\
  --setup-k8s=false \\
  --base origin/main \\
  --head HEAD \\
  --profile standard \\
  --output codex-review/k8s-review.md`,
  },
  {
    label: "Reuse the endpoint",
    value: `export CODEX_REVIEWER_API_URL=${apiUrl}

codex-reviewer review submit \\
  --setup-k8s=false \\
  --base origin/main \\
  --head HEAD`,
  },
];

const checks = [
  ["Clean tree", "Commit or stash local work before submitting."],
  ["Reachable repo", "The Kubernetes job clones the submitted Git remote."],
  ["Private access", "Keep GITHUB_TOKEN available when reviewing private repositories."],
  ["Review output", "Reports are written under codex-review/ with a tracking record."],
];

function App() {
  return (
    <main className="min-h-screen bg-paper text-ink">
      <section className="relative overflow-hidden border-b border-line">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_78%_18%,rgba(31,138,112,0.18),transparent_30%),linear-gradient(120deg,rgba(16,21,28,0.96),rgba(27,38,48,0.92)_46%,rgba(247,247,242,0)_46%)]" />
        <div className="relative mx-auto grid min-h-[92svh] max-w-7xl grid-cols-1 items-center gap-12 px-6 py-10 md:grid-cols-[0.92fr_1.08fr] md:px-10 lg:px-12">
          <div className="animate-rise max-w-2xl text-white">
            <a
              className="inline-flex items-center gap-2 text-sm font-medium text-white/75 transition hover:text-white"
              href={repoUrl}
              rel="noreferrer"
              target="_blank"
            >
              <Github className="size-4" />
              GitHub
              <ExternalLink className="size-3.5" />
            </a>
            <p className="mt-12 font-mono text-sm uppercase tracking-[0.24em] text-white/58">
              Kubernetes review service
            </p>
            <h1 className="mt-5 text-5xl font-semibold leading-[0.96] text-white sm:text-6xl lg:text-7xl">
              Codex Reviewer
            </h1>
            <p className="mt-6 max-w-xl text-lg leading-8 text-white/74">
              Submit a committed branch for an isolated code review job and get
              a Markdown report back in your repository.
            </p>
            <div className="mt-9 flex flex-wrap gap-3">
              <a className="button-primary" href="#run">
                Run a review
                <ArrowRight className="size-4" />
              </a>
              <a className="button-secondary" href={repoUrl} rel="noreferrer" target="_blank">
                View source
                <Github className="size-4" />
              </a>
            </div>
          </div>

          <div className="animate-rise-delay relative">
            <div className="terminal-frame shadow-glow">
              <div className="flex items-center justify-between border-b border-white/10 px-5 py-4">
                <div className="flex gap-2">
                  <span className="size-3 rounded-full bg-ember" />
                  <span className="size-3 rounded-full bg-[#e4b84c]" />
                  <span className="size-3 rounded-full bg-signal" />
                </div>
                <div className="flex items-center gap-2 font-mono text-xs text-white/42">
                  <Globe2 className="size-3.5" />
                  {new URL(apiUrl).host}
                </div>
              </div>
              <div className="p-5 sm:p-7">
                <div className="mb-5 flex items-center gap-3 text-white">
                  <Terminal className="size-5 text-signal" />
                  <span className="font-mono text-sm">review submit</span>
                </div>
                <pre className="command-block whitespace-pre-wrap break-words text-sm leading-7">
                  {commands[0].value}
                </pre>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section id="run" className="mx-auto max-w-7xl px-6 py-16 md:px-10 lg:px-12">
        <div className="grid gap-12 lg:grid-cols-[0.82fr_1.18fr]">
          <div className="reveal">
            <div className="inline-flex items-center gap-2 border-b border-ink pb-2 font-mono text-sm">
              <GitBranch className="size-4" />
              Current branch
            </div>
            <h2 className="mt-7 text-3xl font-semibold leading-tight sm:text-4xl">
              Run it from the repository you want reviewed.
            </h2>
            <p className="mt-5 max-w-lg text-base leading-7 text-steel">
              The service reads the submitted Git ref, starts one Kubernetes
              job, waits for the report, and stores a non-secret tracking
              record beside the report.
            </p>
          </div>

          <div className="space-y-5">
            {commands.map((command, index) => (
              <div className="command-panel reveal" key={command.label} style={{ "--delay": `${index * 80}ms` }}>
                <div className="mb-4 flex items-center justify-between gap-4">
                  <h3 className="font-mono text-sm font-semibold uppercase tracking-[0.18em] text-steel">
                    {command.label}
                  </h3>
                  <CheckCircle2 className="size-5 text-signal" />
                </div>
                <pre className="whitespace-pre-wrap break-words font-mono text-sm leading-7 text-ink">
                  {command.value}
                </pre>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="border-y border-line bg-white">
        <div className="mx-auto grid max-w-7xl gap-10 px-6 py-14 md:grid-cols-[0.7fr_1.3fr] md:px-10 lg:px-12">
          <div className="reveal">
            <div className="inline-flex items-center gap-2 border-b border-ink pb-2 font-mono text-sm">
              <ShieldCheck className="size-4" />
              Before submit
            </div>
            <h2 className="mt-7 text-3xl font-semibold">What the job expects</h2>
          </div>
          <div className="grid gap-x-10 gap-y-8 sm:grid-cols-2">
            {checks.map(([title, body], index) => (
              <div className="reveal border-t border-line pt-5" key={title} style={{ "--delay": `${index * 70}ms` }}>
                <h3 className="text-lg font-semibold">{title}</h3>
                <p className="mt-3 text-sm leading-6 text-steel">{body}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="mx-auto flex max-w-7xl flex-col gap-6 px-6 py-12 md:flex-row md:items-center md:justify-between md:px-10 lg:px-12">
        <div>
          <p className="font-mono text-sm uppercase tracking-[0.2em] text-steel">API status</p>
          <h2 className="mt-3 text-2xl font-semibold">Health endpoints stay available.</h2>
        </div>
        <div className="flex flex-wrap gap-3">
          <a className="link-button" href="/healthz">
            /healthz
            <ArrowRight className="size-4" />
          </a>
          <a className="link-button" href="/readyz">
            /readyz
            <ArrowRight className="size-4" />
          </a>
          <a className="link-button" href={repoUrl} rel="noreferrer" target="_blank">
            GitHub
            <ExternalLink className="size-4" />
          </a>
        </div>
      </section>
    </main>
  );
}

createRoot(document.getElementById("root")).render(<App />);
