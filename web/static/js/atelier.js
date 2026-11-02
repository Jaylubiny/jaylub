(() => {
  const canvas = document.querySelector("[data-ambient-canvas]");
  const motionPreference = window.matchMedia("(prefers-reduced-motion: reduce)");

  if (canvas && !motionPreference.matches) {
    const context = canvas.getContext("2d", { alpha: true });
    if (context) {
      const pointer = { x: -1000, y: -1000 };
      const particles = [];
      const accent = getComputedStyle(document.documentElement).getPropertyValue("--theme-purple").trim();
      let width = 0;
      let height = 0;
      let animationFrame = 0;

      function resizeCanvas() {
        const ratio = Math.min(window.devicePixelRatio || 1, 1.5);
        width = window.innerWidth;
        height = window.innerHeight;
        canvas.width = Math.round(width * ratio);
        canvas.height = Math.round(height * ratio);
        context.setTransform(ratio, 0, 0, ratio, 0, 0);

        const count = Math.min(48, Math.max(20, Math.round((width * height) / 32000)));
        while (particles.length < count) {
          particles.push({
            x: Math.random() * width,
            y: Math.random() * height,
            dx: (Math.random() - 0.5) * 0.22,
            dy: (Math.random() - 0.5) * 0.22,
            radius: Math.random() * 1.5 + 0.5
          });
        }
        particles.length = count;
      }

      function animate() {
        context.clearRect(0, 0, width, height);
        for (let index = 0; index < particles.length; index += 1) {
          const particle = particles[index];
          const mouseDX = pointer.x - particle.x;
          const mouseDY = pointer.y - particle.y;
          const distanceToPointer = Math.hypot(mouseDX, mouseDY);

          if (distanceToPointer < 170 && distanceToPointer > 0) {
            particle.dx -= (mouseDX / distanceToPointer) * 0.0015;
            particle.dy -= (mouseDY / distanceToPointer) * 0.0015;
          }
          particle.dx = Math.max(-0.45, Math.min(0.45, particle.dx * 0.998));
          particle.dy = Math.max(-0.45, Math.min(0.45, particle.dy * 0.998));
          particle.x += particle.dx;
          particle.y += particle.dy;
          if (particle.x < -5) particle.x = width + 5;
          if (particle.x > width + 5) particle.x = -5;
          if (particle.y < -5) particle.y = height + 5;
          if (particle.y > height + 5) particle.y = -5;

          context.beginPath();
          context.arc(particle.x, particle.y, particle.radius, 0, Math.PI * 2);
          context.globalAlpha = 0.48;
          context.fillStyle = accent;
          context.fill();
          context.globalAlpha = 1;

          for (let otherIndex = index + 1; otherIndex < particles.length; otherIndex += 1) {
            const other = particles[otherIndex];
            const distance = Math.hypot(particle.x - other.x, particle.y - other.y);
            if (distance > 112) continue;
            context.beginPath();
            context.moveTo(particle.x, particle.y);
            context.lineTo(other.x, other.y);
            context.strokeStyle = accent;
            context.globalAlpha = (1 - distance / 112) * 0.11;
            context.lineWidth = 0.7;
            context.stroke();
            context.globalAlpha = 1;
          }
        }
        animationFrame = window.requestAnimationFrame(animate);
      }

      window.addEventListener("resize", resizeCanvas, { passive: true });
      window.addEventListener("pointermove", (event) => {
        pointer.x = event.clientX;
        pointer.y = event.clientY;
      }, { passive: true });
      motionPreference.addEventListener("change", (event) => {
        if (event.matches) {
          window.cancelAnimationFrame(animationFrame);
          context.clearRect(0, 0, width, height);
        } else {
          resizeCanvas();
          animationFrame = window.requestAnimationFrame(animate);
        }
      });
      resizeCanvas();
      animationFrame = window.requestAnimationFrame(animate);
    }
  }

  const soundToggle = document.querySelector("[data-sound-toggle]");
  let muted = false;
  let audioContext;

  try {
    muted = localStorage.getItem("jaylubInterfaceMuted") === "true";
  } catch {
    muted = false;
  }

  function updateSoundToggle() {
    if (!soundToggle) return;
    soundToggle.textContent = muted ? "Sound off" : "Sound on";
    soundToggle.setAttribute("aria-pressed", String(!muted));
    soundToggle.setAttribute("aria-label", muted ? "Enable interface sounds" : "Mute interface sounds");
    soundToggle.title = muted ? "Enable interface sounds" : "Mute interface sounds";
  }

  function playClick() {
    if (muted) return;
    const AudioContextConstructor = window.AudioContext || window.webkitAudioContext;
    if (!AudioContextConstructor) return;

    try {
      audioContext ||= new AudioContextConstructor();
      if (audioContext.state === "suspended") void audioContext.resume();

      const oscillator = audioContext.createOscillator();
      const gain = audioContext.createGain();
      const now = audioContext.currentTime;
      oscillator.type = "sine";
      oscillator.frequency.setValueAtTime(640 + Math.random() * 90, now);
      oscillator.frequency.exponentialRampToValueAtTime(430, now + 0.045);
      gain.gain.setValueAtTime(0.0001, now);
      gain.gain.exponentialRampToValueAtTime(0.035, now + 0.006);
      gain.gain.exponentialRampToValueAtTime(0.0001, now + 0.065);
      oscillator.connect(gain);
      gain.connect(audioContext.destination);
      oscillator.start(now);
      oscillator.stop(now + 0.07);
    } catch {
      // Browsers without usable audio support simply omit the optional sound.
    }
  }

  updateSoundToggle();
  soundToggle?.addEventListener("click", () => {
    muted = !muted;
    try {
      localStorage.setItem("jaylubInterfaceMuted", String(muted));
    } catch {
      // Keep the in-memory preference if browser storage is unavailable.
    }
    updateSoundToggle();
  });

  document.addEventListener("click", (event) => {
    const target = event.target instanceof Element ? event.target : null;
    if (!target || target.closest("[data-sound-toggle]")) return;
    if (target.closest("button, a, [role='button'], .card, .feature-card")) playClick();
  });

  document.addEventListener("click", (event) => {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey ||
        event.ctrlKey || event.shiftKey || event.altKey) return;
    const target = event.target instanceof Element ? event.target : null;
    const link = target?.closest("a[href]");
    if (!link || link.hasAttribute("download") || link.target) return;

    let destination;
    try {
      destination = new URL(link.href, window.location.href);
    } catch {
      return;
    }
    if (destination.origin !== window.location.origin ||
        destination.pathname === window.location.pathname + destination.hash) return;

    event.preventDefault();
    document.body.classList.add("is-leaving");
    window.setTimeout(() => window.location.assign(destination.href), 130);
  });
})();
