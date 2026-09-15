/*
 * Factory Showroom - editable content.
 *
 * ALL TEXT BELOW IS PLACEHOLDER CONTENT. Replace the values with real
 * biography data before using this page as a real CV. Every placeholder
 * string is prefixed with [PLACEHOLDER] so nothing ships by accident.
 *
 * English only. No external network calls. Plain data, no framework.
 */
window.SHOWROOM_CONTENT = {
  meta: {
    title: 'Factory Showroom',
    tagline: 'Assembly line for platform engineering',
    version: 'v0.1.0'
  },

  person: {
    name: 'Bruner',
    role: 'DevOps / Platform Engineer',
    location: '[PLACEHOLDER] Togliatti, Russia',
    availability: '[PLACEHOLDER] Open to platform engineering work',
    summary:
      '[PLACEHOLDER] Platform engineer who treats delivery like a production line: ' +
      'every change is inspected, assembled, quality-checked and only then moved to the showroom. ' +
      'Works on single-node Kubernetes distributions, controllers, CI pipelines and developer tooling.',
    stats: [
      { label: 'Years in operations', value: 8, suffix: '+' },
      { label: 'Clusters operated', value: 24, suffix: '' },
      { label: 'Pipelines shipped', value: 130, suffix: '+' },
      { label: 'Incidents post-mortemed', value: 47, suffix: '' }
    ]
  },

  /* The four assembly line stages. Order is fixed and meaningful. */
  stages: [
    {
      id: 'inspection',
      code: 'ST-01',
      name: 'Inspection',
      caption: 'Preflight checks, policy validation, image reference review.'
    },
    {
      id: 'assembly',
      code: 'ST-02',
      name: 'Assembly',
      caption: 'Manifests applied, Deployment and Service assembled by the controller.'
    },
    {
      id: 'quality-control',
      code: 'ST-03',
      name: 'Quality Control',
      caption: 'Readiness probes, rollout status, quality gate verdict.'
    },
    {
      id: 'showroom',
      code: 'ST-04',
      name: 'Showroom',
      caption: 'All replicas ready. Product cleared for the showroom floor.'
    }
  ],

  skillGroups: [
    {
      name: 'Orchestration',
      items: [
        { label: '[PLACEHOLDER] Kubernetes', level: 92 },
        { label: '[PLACEHOLDER] containerd / OCI', level: 80 },
        { label: '[PLACEHOLDER] Helm', level: 85 },
        { label: '[PLACEHOLDER] controller-runtime', level: 74 }
      ]
    },
    {
      name: 'Automation',
      items: [
        { label: '[PLACEHOLDER] Terraform', level: 82 },
        { label: '[PLACEHOLDER] Ansible', level: 78 },
        { label: '[PLACEHOLDER] GitHub Actions', level: 88 },
        { label: '[PLACEHOLDER] Bash / Make', level: 90 }
      ]
    },
    {
      name: 'Languages and observability',
      items: [
        { label: '[PLACEHOLDER] Go', level: 76 },
        { label: '[PLACEHOLDER] Python', level: 81 },
        { label: '[PLACEHOLDER] Prometheus / Grafana', level: 84 },
        { label: '[PLACEHOLDER] Loki / OpenTelemetry', level: 70 }
      ]
    }
  ],

  experience: [
    {
      period: '[PLACEHOLDER] 2023 - present',
      role: '[PLACEHOLDER] Platform Engineer',
      org: '[PLACEHOLDER] Independent',
      stage: 'Showroom',
      notes: [
        '[PLACEHOLDER] Designed Brunernetes, a single-node Kubernetes distribution with themed diagnostics.',
        '[PLACEHOLDER] Built four custom controllers on controller-runtime with envtest coverage.'
      ]
    },
    {
      period: '[PLACEHOLDER] 2020 - 2023',
      role: '[PLACEHOLDER] Senior DevOps Engineer',
      org: '[PLACEHOLDER] Manufacturing group',
      stage: 'Quality Control',
      notes: [
        '[PLACEHOLDER] Introduced quality gates that blocked releases without readiness probes.',
        '[PLACEHOLDER] Cut mean rollout time from 34 minutes to 6 minutes.'
      ]
    },
    {
      period: '[PLACEHOLDER] 2018 - 2020',
      role: '[PLACEHOLDER] Infrastructure Engineer',
      org: '[PLACEHOLDER] Logistics platform',
      stage: 'Assembly',
      notes: [
        '[PLACEHOLDER] Migrated 40 services from virtual machines to containers.',
        '[PLACEHOLDER] Standardised infrastructure as code across three environments.'
      ]
    },
    {
      period: '[PLACEHOLDER] 2016 - 2018',
      role: '[PLACEHOLDER] Linux Systems Administrator',
      org: '[PLACEHOLDER] Regional ISP',
      stage: 'Inspection',
      notes: [
        '[PLACEHOLDER] Ran preflight and capacity checks for on-premise hardware.',
        '[PLACEHOLDER] Automated host provisioning and backup verification.'
      ]
    }
  ],

  projects: [
    {
      name: '[PLACEHOLDER] Brunernetes',
      kind: 'Single-node distribution',
      summary:
        '[PLACEHOLDER] Kubernetes distribution and developer experience layer for one Linux host, ' +
        'with CRDs for Togliatti, AvtoVAZ, Konveer and BrunerApp.',
      tags: ['Go', 'Kubebuilder', 'Helm', 'kubeadm']
    },
    {
      name: '[PLACEHOLDER] brunerctl',
      kind: 'Command line tool',
      summary:
        '[PLACEHOLDER] Cobra based CLI for install, status, app deploy, diagnose and uninstall. ' +
        'Every message states the technical condition first.',
      tags: ['Go', 'Cobra', 'client-go']
    },
    {
      name: '[PLACEHOLDER] Factory Showroom',
      kind: 'Demo application',
      summary:
        '[PLACEHOLDER] This page. A static, offline capable portfolio served by nginx on port 8080 ' +
        'and deployed by a BrunerApp resource.',
      tags: ['HTML', 'CSS', 'nginx']
    },
    {
      name: '[PLACEHOLDER] Line Telemetry',
      kind: 'Observability stack',
      summary:
        '[PLACEHOLDER] Dashboards and alert rules that map rollout phases onto the four assembly stages.',
      tags: ['Prometheus', 'Grafana', 'Loki']
    }
  ],

  contact: {
    note:
      '[PLACEHOLDER] Replace these values with real contact details. ' +
      'This page performs no network requests, so nothing is submitted anywhere.',
    channels: [
      { label: 'Email', value: '[PLACEHOLDER] bruner@example.com', href: 'mailto:bruner@example.com' },
      { label: 'Git', value: '[PLACEHOLDER] git.example.com/bruner', href: null },
      { label: 'Location', value: '[PLACEHOLDER] Togliatti, Russia', href: null },
      { label: 'Response time', value: '[PLACEHOLDER] Within two working days', href: null }
    ]
  }
};
