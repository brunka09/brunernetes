# Factory Showroom - QA notes

Verified in a Chromium browser against the deployed static bundle of `site/`
(`python3 -m http.server` inside the sandbox is not reachable from the cloud browser,
so the same files were served over the preview host instead - identical content).

Checked:

- Layout at 1440 px, 768 px, 375 px: no horizontal overflow (`scrollWidth` equals the
  viewport width at every breakpoint), no clipped or overlapping text.
- Assembly line: car body travels Inspection -> Assembly -> Quality Control ->
  Showroom, then exits right and re-enters from the left. Stage bar, gantry lamp,
  stage-specific car treatment and the `aria-live` readout all follow the active stage.
- `Pause line` / `Resume line` toggle updates `aria-pressed` and stops/starts
  `requestAnimationFrame` correctly.
- `prefers-reduced-motion: reduce` (emulated): all animation disabled, car parked in
  the Showroom bay, control hidden, hint text explains why.
- Console: no errors or warnings (`Log.entryAdded` / `Runtime.exceptionThrown` empty
  after a full reload).
- HTML: tags balanced, no external `http(s)` `src`/`href` references anywhere, no
  non-ASCII characters and no emojis in any source file.
- `site/healthz` contains exactly the two bytes `ok` (no trailing newline).

Not verifiable in this sandbox: `docker build`/`docker run` and `nginx -t` - neither
docker nor nginx is installed here. The Dockerfile runs `nginx -t -c /etc/nginx/nginx.conf`
at build time, so a malformed config fails the build rather than the container start.

Screenshots in this folder: 1440 px hero / line / experience / projects / contact,
768 px skills, 375 px hero / line, and the reduced-motion state.
