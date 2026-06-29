const canvas = document.querySelector("#edge-particles");
const ctx = canvas.getContext("2d", { alpha: true });

const config = {
  count: 150,
  edgeDepth: 150,
  maxSpeed: 0.22,
  blue: [53, 208, 255],
  red: [53, 208, 255],
  yellow: [53, 208, 255],
  green: [53, 208, 255],
};

let width = 0;
let height = 0;
let dpr = 1;
let particles = [];
let particleState = {
  color: config.blue,
  about: 0,
  services: 0,
  contact: 0,
};
let particleTarget = {
  color: config.blue,
  about: 0,
  services: 0,
  contact: 0,
};

function randomBetween(min, max) {
  return min + Math.random() * (max - min);
}

function clamp(value, min = 0, max = 1) {
  return Math.max(min, Math.min(max, value));
}

function mixColor(from, to, amount) {
  return from.map((channel, index) => {
    return Math.round(channel + (to[index] - channel) * amount);
  });
}

function easeNumber(from, to, amount) {
  return from + (to - from) * amount;
}

function colorString(color) {
  return color.join(", ");
}

function createParticle() {
  const side = Math.floor(Math.random() * 4);
  let x = 0;
  let y = 0;

  if (side === 0) {
    x = randomBetween(0, width);
    y = randomBetween(0, config.edgeDepth);
  } else if (side === 1) {
    x = randomBetween(width - config.edgeDepth, width);
    y = randomBetween(0, height);
  } else if (side === 2) {
    x = randomBetween(0, width);
    y = randomBetween(height - config.edgeDepth, height);
  } else {
    x = randomBetween(0, config.edgeDepth);
    y = randomBetween(0, height);
  }

  return {
    x,
    y,
    vx: randomBetween(-config.maxSpeed, config.maxSpeed),
    vy: randomBetween(-config.maxSpeed, config.maxSpeed),
    radius: randomBetween(0.8, 2.1),
    alpha: randomBetween(0.16, 0.72),
    pulse: randomBetween(0, Math.PI * 2),
  };
}

function resize() {
  dpr = Math.min(window.devicePixelRatio || 1, 2);
  width = window.innerWidth;
  height = window.innerHeight;
  canvas.width = Math.floor(width * dpr);
  canvas.height = Math.floor(height * dpr);
  canvas.style.width = `${width}px`;
  canvas.style.height = `${height}px`;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  particles = Array.from({ length: config.count }, createParticle);
  updateParticleState();
}

function updateParticleState() {
  const activeSection = document.body.dataset.activeSection || "home";

  if (document.querySelector(".contact-page")) {
    particleTarget = {
      color: config.green,
      about: 0,
      services: 0,
      contact: 1,
    };
  } else if (activeSection === "about") {
    particleTarget = {
      color: config.yellow,
      about: 1,
      services: 0,
      contact: 0,
    };
  } else if (activeSection === "services") {
    particleTarget = {
      color: config.red,
      about: 0,
      services: 1,
      contact: 0,
    };
  } else if (activeSection === "contact") {
    particleTarget = {
      color: config.green,
      about: 0,
      services: 0,
      contact: 1,
    };
  } else {
    particleTarget = {
      color: config.blue,
      about: 0,
      services: 0,
      contact: 0,
    };
  }

  particleState = {
    color: particleState.color.map((channel, index) => {
      return Math.round(easeNumber(channel, particleTarget.color[index], 0.08));
    }),
    about: clamp(easeNumber(particleState.about, particleTarget.about, 0.08)),
    services: clamp(easeNumber(particleState.services, particleTarget.services, 0.08)),
    contact: clamp(easeNumber(particleState.contact, particleTarget.contact, 0.08)),
  };
}

function edgeForce(particle) {
  const edgeDepth = config.edgeDepth + particleState.about * 80 + particleState.services * 110 + particleState.contact * 60;
  const nearestEdge = Math.min(
    particle.x,
    width - particle.x,
    particle.y,
    height - particle.y,
  );

  if (nearestEdge > edgeDepth) {
    const edgePush = 1 - particleState.about * 0.45 - particleState.services * 0.2 - particleState.contact * 0.3;
    particle.vx += (particle.x < width / 2 ? -0.008 : 0.008) * edgePush;
    particle.vy += (particle.y < height / 2 ? -0.008 : 0.008) * edgePush;
  }
}

function aboutForce(particle) {
  if (particleState.about <= 0) {
    return;
  }

  const dx = particle.x - width / 2;
  const dy = particle.y - height / 2;
  const distance = Math.max(1, Math.hypot(dx, dy));
  const orbit = 0.006 * particleState.about;
  const pull = 0.0025 * particleState.about;

  particle.vx += (-dy / distance) * orbit - (dx / distance) * pull;
  particle.vy += (dx / distance) * orbit - (dy / distance) * pull;
}

function servicesForce(particle) {
  if (particleState.services <= 0) {
    return;
  }

  const column = Math.sin(particle.x * 0.024 + particle.pulse) * 0.009 * particleState.services;
  const lane = Math.sin((particle.y / Math.max(height, 1)) * Math.PI * 4 + particle.pulse) * 0.004 * particleState.services;
  const centerDrift = ((height / 2 - particle.y) / Math.max(height, 1)) * 0.008 * particleState.services;

  particle.vx += lane;
  particle.vy += column + centerDrift;
}

function contactForce(particle) {
  if (particleState.contact <= 0) {
    return;
  }

  const wave = Math.sin(particle.y * 0.018 + particle.pulse) * 0.006 * particleState.contact;
  const centerPull = ((width / 2 - particle.x) / Math.max(width, 1)) * 0.012 * particleState.contact;

  particle.vx += wave + centerPull;
  particle.vy -= 0.011 * particleState.contact;

  if (particle.y < -12 && Math.random() < 0.08 * particleState.contact) {
    particle.y = height + randomBetween(0, config.edgeDepth);
    particle.x = randomBetween(0, width);
  }
}

function updateParticle(particle) {
  particle.x += particle.vx;
  particle.y += particle.vy;
  particle.pulse += 0.018 + particleState.about * 0.014 + particleState.contact * 0.02;

  edgeForce(particle);
  aboutForce(particle);
  servicesForce(particle);
  contactForce(particle);

  const drift = 0.01 + particleState.about * 0.008 + particleState.services * 0.011 + particleState.contact * 0.014;
  const maxSpeed = config.maxSpeed + particleState.about * 0.08 + particleState.services * 0.12 + particleState.contact * 0.16;
  particle.vx += randomBetween(-drift, drift);
  particle.vy += randomBetween(-drift, drift);
  particle.vx = Math.max(-maxSpeed, Math.min(maxSpeed, particle.vx));
  particle.vy = Math.max(-maxSpeed, Math.min(maxSpeed, particle.vy));

  if (
    particle.x < -20 ||
    particle.x > width + 20 ||
    particle.y < -20 ||
    particle.y > height + 20
  ) {
    Object.assign(particle, createParticle());
  }
}

function drawParticle(particle) {
  const glow = particle.alpha + Math.sin(particle.pulse) * 0.12;
  ctx.beginPath();
  ctx.arc(
    particle.x,
    particle.y,
    particle.radius + particleState.about * 0.35 + particleState.services * 0.25 + particleState.contact * 0.55,
    0,
    Math.PI * 2,
  );
  ctx.fillStyle = `rgba(${colorString(particleState.color)}, ${Math.max(0.08, glow)})`;
  ctx.fill();
}

function drawConnections() {
  for (let i = 0; i < particles.length; i += 1) {
    for (let j = i + 1; j < particles.length; j += 1) {
      const a = particles[i];
      const b = particles[j];
      const dx = a.x - b.x;
      const dy = a.y - b.y;
      const distance = Math.hypot(dx, dy);

      const connectionDistance = 90 + particleState.about * 54 + particleState.services * 28 - particleState.contact * 18;

      if (distance < connectionDistance) {
        ctx.beginPath();
        ctx.moveTo(a.x, a.y);
        ctx.lineTo(b.x, b.y);
        ctx.strokeStyle = `rgba(${colorString(particleState.color)}, ${0.08 * (1 - distance / connectionDistance)})`;
        ctx.lineWidth = 1;
        ctx.stroke();
      }
    }
  }
}

function animate() {
  updateParticleState();
  ctx.clearRect(0, 0, width, height);
  const color = colorString(particleState.color);

  const gradient = ctx.createRadialGradient(
    width / 2,
    height / 2,
    Math.min(width, height) * 0.1,
    width / 2,
    height / 2,
    Math.max(width, height) * 0.7,
  );
  gradient.addColorStop(0, "rgba(3, 3, 3, 0)");
  gradient.addColorStop(1, `rgba(${color}, ${0.08 + particleState.about * 0.05 + particleState.services * 0.06 + particleState.contact * 0.05})`);
  ctx.fillStyle = gradient;
  ctx.fillRect(0, 0, width, height);

  particles.forEach(updateParticle);
  drawConnections();
  particles.forEach(drawParticle);

  window.requestAnimationFrame(animate);
}

window.addEventListener("resize", resize);
window.addEventListener("sectionchange", updateParticleState);
resize();
animate();
