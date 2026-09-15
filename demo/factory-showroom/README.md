# Factory Showroom

Factory Showroom is the single demo web application of Brunernetes. It is a static,
English-only page that presents an animated CV / DevOps portfolio for a platform
engineer and visualises the delivery pipeline as a factory assembly line with four
fixed stages:

```
Inspection  ->  Assembly  ->  Quality Control  ->  Showroom
```

A car body travels from stage to stage on a conveyor. Each stage lights up, changes
the body treatment (inspection scan, assembly sparks, quality verdict, showroom
spotlight) and updates a technical readout line. The same four stages are the mental
model used by the Brunernetes controllers: a change is inspected, assembled into a
Deployment and Service, quality-gated by readiness probes, and only then cleared for
the showroom floor.

## What is inside

```
demo/factory-showroom/
  Dockerfile        nginx:1.27-alpine image, non-root (uid 101), port 8080
  nginx.conf        minimal server: port 8080, gzip, security headers, /healthz
  README.md         this file
  site/
    index.html      semantic HTML5 single page
    styles.css      industrial dark steel theme, responsive, reduced-motion aware
    app.js          rendering plus the requestAnimationFrame assembly line
    content.js      ALL editable CV content in one plain data object
    healthz         static file containing exactly "ok"
```

Properties that matter for the MVP:

- No frameworks, no CDN, no fonts fetched over the network, no analytics, no
  tracking, no outbound requests of any kind. The page works fully offline.
- No emojis, English only, in the page and in every source comment.
- Accessible: semantic landmarks, skip link, labelled SVG, `aria-live` stage
  readout, visible focus rings, and `prefers-reduced-motion` support that disables
  every animation and parks the car body in the Showroom stage.
- Responsive and verified at 375 px, 768 px and 1440 px viewport widths.

## Editing the content

All biography text lives in `site/content.js` in the `window.SHOWROOM_CONTENT`
object. Every string is placeholder text prefixed with `[PLACEHOLDER]`; replace the
values and reload. The four assembly line stages are data driven too, but their
order and count are fixed by the specification - do not change them.

## Run locally without Docker

```bash
cd demo/factory-showroom/site
python3 -m http.server 8080
# open http://localhost:8080/
curl -s http://localhost:8080/healthz   # -> ok
```

Note that `python3 -m http.server` only serves the files; the `/healthz` behaviour,
gzip and the security headers come from `nginx.conf`.

## Build and run with Docker

```bash
cd demo/factory-showroom
docker build -t factory-showroom:0.1.0 .
docker run --rm -p 8080:8080 --read-only --tmpfs /tmp factory-showroom:0.1.0
```

Verify:

```bash
curl -s http://localhost:8080/            | head -n 5
curl -si http://localhost:8080/healthz    # 200, content-type text/plain, body "ok"
docker inspect --format '{{.State.Health.Status}}' <container-id>
```

The image runs as uid 101 (`nginx`), listens on 8080 so no privileged port is
needed, writes its PID and temp files under `/tmp`, hides the nginx version
(`server_tokens off`) and declares a `HEALTHCHECK` against `/healthz`.

## How Brunernetes consumes it

Factory Showroom is deployed by a `BrunerApp` resource; the `BrunerApp` controller
owns the resulting Deployment and Service and applies the probe and resource
defaults. The image is built and pushed outside the cluster - Brunernetes never
builds images - and `Konveer` only validates and rolls out an existing reference.

```yaml
apiVersion: bruner.dev/v1alpha1
kind: BrunerApp
metadata:
  name: factory-showroom
  namespace: brunernetes-demo
spec:
  image: factory-showroom:0.1.0
  imagePullPolicy: IfNotPresent
  port: 8080
  replicas: 1
  healthPath: /healthz
  mode:
    profile: showroom
    rollout: steady
    qualityGate: practical
    shift: day
  localExposure:
    enabled: true
    hostIP: 127.0.0.1
    hostPort: 30080
```

The same manifest is checked in at `demo/manifests/brunerapp.yaml`.

The readiness and liveness probes both target `/healthz`, which is why that endpoint
must stay a plain `200` with the body `ok` and content type `text/plain`. Once all
replicas report ready, the controller reports the themed but technically explicit
condition: "All replicas are ready. Assembly completed and product cleared for the
showroom." The application is exposed on localhost only; there is no ingress.

A typical local flow:

```bash
brunerctl install --preset garage
brunerctl app deploy -f demo/manifests/brunerapp.yaml
brunerctl app inspect factory-showroom -n brunernetes-demo
brunerctl status
```

## Future v0.2: Build the Konveer mini-game

Out of scope for the MVP, documented here so the idea is not lost.

**Concept.** "Build the Konveer" turns the static assembly line into a small,
self-contained browser puzzle game. The player is a line engineer who must lay down a
conveyor that routes incoming car bodies through the four stages in the correct order
- Inspection, Assembly, Quality Control, Showroom - before the shift ends.

**Mechanics.**

- A grid shop floor. The player places conveyor segments (straight, corner, merge)
  and the four stage machines from a limited parts budget.
- Car bodies spawn at the loading dock on a timer and move automatically along the
  belt. A body that reaches a stage out of order is rejected and returns to the dock,
  which costs shift time.
- Each stage applies a check that mirrors the real controllers: Inspection rejects
  bodies with an invalid part reference, Assembly needs a free worker slot, Quality
  Control fails bodies without a readiness stamp, Showroom scores the finished car.
- Shift modifiers reuse the `BrunerMode` profiles: `garage` is forgiving, `strict`
  requires a readiness stamp on every body, `friday` randomly injects an override
  prompt before a rollout, `showroom` runs a presentation mode with clear statuses.
- Scoring rewards throughput and penalises rejects; a short end-of-shift report is
  written in the same "technical condition first" wording as `brunerctl`.

**Scope for v0.2.** Pure client-side: HTML, CSS, Canvas 2D and vanilla JavaScript in
the same image, no backend, no network calls, no accounts, no leaderboard. Target
around 5 hand-designed levels, keyboard and pointer input, an accessible
non-interactive fallback that keeps the current animated line for reduced-motion and
screen reader users, and no increase in the container's attack surface.

**Why it is out of MVP scope.** The MVP has to prove the distribution: kubeadm
bootstrap, four CRDs, controllers, the `brunerctl` CLI and one reliably deployable
demo application. A game needs its own state machine, level data, balancing, input
handling and test approach, which is a project of its own and adds risk to the parts
that actually validate Brunernetes. The current page already demonstrates the four
stages, exposes `/healthz`, stays tiny and can be served from a read-only container -
everything the platform needs from a demo workload. The game becomes worthwhile only
after the platform itself is stable.
