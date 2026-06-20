(() => {
  const canvas = document.getElementById("survivorCanvas");
  const ctx = canvas.getContext("2d");
  const stage = document.querySelector(".survivor-stage");
  const hud = document.getElementById("gameHud");
  const fullscreenButtons = document.querySelectorAll("[data-fullscreen-button]");
  const goldLabels = document.querySelectorAll("[data-gold]");
  const lifetimeKillLabels = document.querySelectorAll("[data-lifetime-kills]");
  const shopCategoryButtons = document.querySelectorAll("[data-shop-category]");
  const leaderboardSortButtons = document.querySelectorAll("[data-leaderboard-sort]");
  const menuBackButtons = document.querySelectorAll("[data-menu-back]");
  const screens = {
    menu: document.getElementById("mainMenu"),
    shop: document.getElementById("shopScreen"),
    leaderboard: document.getElementById("leaderboardScreen"),
    gameOver: document.getElementById("gameOverScreen"),
    pause: document.getElementById("pauseScreen"),
  };

  const ui = {
    hpFill: document.getElementById("hpFill"),
    hpText: document.getElementById("hpText"),
    runGold: document.getElementById("runGold"),
    runKills: document.getElementById("runKills"),
    runLevel: document.getElementById("runLevel"),
    xpFill: document.getElementById("xpFill"),
    xpText: document.getElementById("xpText"),
    runTime: document.getElementById("runTime"),
    bossPanel: document.getElementById("bossPanel"),
    bossFill: document.getElementById("bossFill"),
    bossText: document.getElementById("bossText"),
    finalTime: document.getElementById("finalTime"),
    finalKills: document.getElementById("finalKills"),
    finalGold: document.getElementById("finalGold"),
    characterList: document.getElementById("characterList"),
    upgradeList: document.getElementById("upgradeList"),
    leaderboardRows: document.getElementById("leaderboardRows"),
    leaderboardEyebrow: document.getElementById("leaderboardEyebrow"),
    leaderboardValueHeader: document.getElementById("leaderboardValueHeader"),
  };

  const playerImage = new Image();
  playerImage.src = "/static/img/character.png";
  const goblinImage = new Image();
  goblinImage.src = "/static/img/character2.png";
  const character3Image = new Image();
  character3Image.src = "/static/img/character3.png";
  const enemyImage = new Image();
  enemyImage.src = "/static/img/enemy.png";
  const enemy2Image = new Image();
  enemy2Image.src = "/static/img/enemy2.png";
  const healImage = new Image();
  healImage.src = "/static/img/heal.png";
  const footballImage = new Image();
  footballImage.src = "/static/img/football.png";
  const bikeImage = new Image();
  bikeImage.src = "/static/img/bike.png";
  const pigeonImage = new Image();
  pigeonImage.src = "/static/img/pigeon.png";
  const bossImages = [new Image(), new Image(), new Image()];
  bossImages[0].src = "/static/img/boss.png";
  bossImages[1].src = "/static/img/boss2.png";
  bossImages[2].src = "/static/img/boss3.png";

  const bossConfigs = [
    { name: "Boss 1", hp: 500, reward: 250, imageIndex: 0, r: 66, size: 138, speed: 54, cooldown: 1.8, shotSpeed: 230, damage: 14, burstShots: 12 },
    { name: "Boss 2", hp: 1500, reward: 500, imageIndex: 1, r: 80, size: 168, speed: 62, cooldown: 1.35, shotSpeed: 260, damage: 18, burstShots: 14 },
    { name: "Boss 3", hp: 3000, reward: 1000, imageIndex: 2, r: 94, size: 198, speed: 70, cooldown: 1.05, shotSpeed: 290, damage: 24, burstShots: 16 },
  ];
  const bossSpawnGraceSeconds = 90;
  const itemPool = [
    { id: "aura", name: "Aura", rarity: "Common", weight: 55, color: "#7fb069" },
    { id: "football", name: "Football", rarity: "Rare", weight: 25, color: "#f1ead7" },
    { id: "bike", name: "Bike", rarity: "Epic", weight: 15, color: "#5992a7" },
    { id: "pigeon", name: "Pigeon", rarity: "Legendary", weight: 5, color: "#d5a43a" },
  ];
  const boostPool = [
    { id: "damage", name: "+5% Damage", color: "#d5a43a", apply: (p) => { p.runDamageMult += 0.05; } },
    { id: "attackSpeed", name: "+2% Attack Speed", color: "#f1ead7", apply: (p) => { p.runAttackSpeedMult += 0.02; } },
    { id: "maxHp", name: "+5% Max Health", color: "#9bd27b", apply: (p) => {
      const gain = Math.ceil(p.baseMaxHp * 0.05);
      p.runMaxHpBonus += gain;
      p.hp += gain;
    } },
    { id: "moveSpeed", name: "+3% Move Speed", color: "#5992a7", apply: (p) => { p.runSpeedMult += 0.03; } },
  ];

  const keys = new Set();
  const mouse = { x: 0, y: 0, worldX: 0, worldY: 0 };
  const world = { width: 2600, height: 2000 };
  const camera = { x: 0, y: 0 };
  const viewport = { width: 960, height: 540, dpr: window.devicePixelRatio || 1, left: 0, top: 0 };
  const perf = {
    enemyCellSize: 256,
    drawPadding: 160,
    maxParticles: 360,
    maxNumbers: 120,
  };
  const enemyGrid = new Map();
  const rngDetails = makeEnvironment();
  const worldCanvas = makeWorldCanvas();
  const state = {
    mode: "menu",
    profile: null,
    shop: {},
    leaderboard: [],
    leaderboardSort: "totalKills",
    shopCategory: "stats",
    running: false,
    paused: false,
    pausedAt: 0,
    lastTime: performance.now(),
    spawnTimer: 0,
    healSpawnTimer: 0,
    spawnPauseTimer: 0,
    nextBossAt: 180,
    bossEncounter: 0,
    runToken: "",
    itemChestTimer: 60,
    boostChestTimer: 30,
    hitPause: 0,
    shake: 0,
    dying: false,
    deathTimer: 0,
    submittedGameOver: false,
  };
  const hudCache = {
    hpWidth: "",
    hpText: "",
    gold: "",
    kills: "",
    level: "",
    xpWidth: "",
    xpText: "",
    time: "",
    bossVisible: false,
    bossWidth: "",
    bossText: "",
  };

  let player;
  let enemies = [];
  let goldDrops = [];
  let healDrops = [];
  let particles = [];
  let numbers = [];
  let slashes = [];
  let projectiles = [];
  let boss;
  let bossProjectiles = [];
  let treasureChests = [];
  let itemChests = [];
  let boostChests = [];
  let nextEnemyId = 1;
  let loopHandle = 0;
  let loopActive = false;
  let runStatsDirty = true;
  let cachedDifficulty = null;
  let cachedDifficultyAt = -1;

  function resizeCanvas() {
    const rect = stage.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    viewport.dpr = dpr;
    viewport.width = Math.max(640, Math.floor(rect.width));
    viewport.height = Math.max(360, Math.floor(rect.height));
    updateCanvasBounds();
    canvas.width = Math.max(640, Math.floor(rect.width * dpr));
    canvas.height = Math.max(360, Math.floor(rect.height * dpr));
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.imageSmoothingEnabled = false;
  }

  function viewWidth() {
    return viewport.width;
  }

  function viewHeight() {
    return viewport.height;
  }

  function updateCanvasBounds() {
    const rect = canvas.getBoundingClientRect();
    viewport.left = rect.left;
    viewport.top = rect.top;
  }

  function showScreen(name) {
    for (const [screenName, element] of Object.entries(screens)) {
      element.hidden = screenName !== name;
    }
    hud.hidden = name !== "game" && name !== "pause";
    state.mode = name;
  }

  async function loadState() {
    const response = await fetch("/game/jaylive/state", { headers: { Accept: "application/json" } });
    if (!response.ok) throw new Error("Could not load game state.");
    const data = await response.json();
    state.profile = data.profile;
    state.shop = data.shop || {};
    state.leaderboard = data.leaderboard || [];
    renderMeta();
  }

  function renderMeta() {
    goldLabels.forEach((el) => {
      el.textContent = state.profile?.gold ?? 0;
    });
    lifetimeKillLabels.forEach((el) => {
      el.textContent = state.profile?.lifetimeKills ?? 0;
    });
    renderCharacters();
    renderShop();
    renderLeaderboard();
  }

  function renderCharacters() {
    const profile = state.profile || {};
    const characters = [
      {
        id: "jaylub",
        name: "Jaylub",
        image: "/static/img/character.png",
        detail: "Auto melee slash",
        unlocked: true,
        cost: 0,
      },
      {
        id: "goblin_jaylub",
        name: "Goblin Jaylub",
        image: "/static/img/character2.png",
        detail: "Shoots projectiles",
        unlocked: Boolean(profile.goblinJaylubUnlocked),
        cost: 100,
      },
      {
        id: "character3",
        name: "Vampire Jaylub",
        image: "/static/img/character3.png",
        detail: "Throws a triple spread of lifesteal knives",
        unlocked: Boolean(profile.vampireJaylubUnlocked),
        cost: 300,
      },
    ];

    ui.characterList.textContent = "";
    for (const character of characters) {
      const selected = profile.selectedCharacter === character.id;
      const slot = document.createElement("div");
      slot.className = "character-slot";
      slot.classList.toggle("active", selected);

      const image = document.createElement("img");
      image.src = character.image;
      image.alt = character.name;

      const copy = document.createElement("div");
      const title = document.createElement("strong");
      title.textContent = character.name;
      const detail = document.createElement("span");
      detail.textContent = selected ? "Selected" : character.detail;
      copy.append(title, detail);

      const button = document.createElement("button");
      button.className = "pixel-button small";
      button.type = "button";
      if (selected) {
        button.textContent = "Selected";
        button.disabled = true;
      } else if (character.unlocked) {
        button.textContent = "Select";
        button.addEventListener("click", () => setCharacter("select", character.id));
      } else {
        button.textContent = `${character.cost} gold`;
        button.disabled = (profile.gold || 0) < character.cost;
        button.addEventListener("click", () => setCharacter("buy", character.id));
      }

      slot.append(image, copy, button);
      ui.characterList.append(slot);
    }
  }

  function renderShop() {
    const upgrades = shopUpgrades();
    shopCategoryButtons.forEach((button) => {
      const active = button.dataset.shopCategory === state.shopCategory;
      button.classList.toggle("is-active", active);
      button.classList.toggle("muted", !active);
    });

    ui.upgradeList.textContent = "";
    for (const [id, name, detail] of upgrades) {
      const meta = state.shop[id] || { level: 0, cost: 35 };
      const row = document.createElement("div");
      row.className = "upgrade-row";

      const copy = document.createElement("div");
      const title = document.createElement("strong");
      title.textContent = name;
      const sub = document.createElement("span");
      const maxText = meta.max ? ` / ${meta.max}` : "";
      sub.textContent = `Level ${meta.level}${maxText} - ${detail}`;
      copy.append(title, sub);

      const button = document.createElement("button");
      button.className = "pixel-button small";
      button.type = "button";
      const maxed = Boolean(meta.max && meta.level >= meta.max);
      button.textContent = maxed ? "Maxed" : `${meta.cost} gold`;
      button.disabled = maxed || (state.profile?.gold ?? 0) < meta.cost;
      button.addEventListener("click", () => buyUpgrade(id));

      row.append(copy, button);
      ui.upgradeList.append(row);
    }
  }

  function shopUpgrades() {
    if (state.shopCategory === "abilities") {
      return [
        ["auraDamage", "Aura Damage", "Common - +12% Aura damage per level"],
        ["footballDamage", "Football Damage", "Rare - +12% Football damage per level"],
        ["bikeDamage", "Bike Damage", "Epic - +12% Bike damage per level"],
        ["pigeonDamage", "Pigeon Damage", "Legendary - +12% Pigeon damage per level"],
      ];
    }

    const upgrades = [
      ["damage", "Damage", "+2 damage per level"],
      ["maxHp", "Max HP", "+12 maximum HP per level"],
      ["attackSpeed", "Attack Speed", "Faster automatic attacks"],
      ["moveSpeed", "Move Speed", "Faster movement"],
    ];
    if (state.profile?.selectedCharacter === "goblin_jaylub") {
      upgrades.push(["piercing", "Piercing", "Goblin projectiles pass through more enemies"]);
    }
    return upgrades;
  }

  function renderLeaderboard() {
    const meta = leaderboardMeta(state.leaderboardSort);
    ui.leaderboardEyebrow.textContent = meta.eyebrow;
    ui.leaderboardValueHeader.textContent = meta.label;
    leaderboardSortButtons.forEach((button) => {
      const active = button.dataset.leaderboardSort === state.leaderboardSort;
      button.classList.toggle("is-active", active);
      button.classList.toggle("muted", !active);
    });

    ui.leaderboardRows.textContent = "";
    if (!state.leaderboard.length) {
      const row = document.createElement("tr");
      const cell = document.createElement("td");
      cell.colSpan = 3;
      cell.textContent = "No runs yet.";
      row.append(cell);
      ui.leaderboardRows.append(row);
      return;
    }
    for (const entry of state.leaderboard) {
      const row = document.createElement("tr");
      const value = meta.value(entry);
      for (const cellValue of [entry.rank, entry.username, value]) {
        const cell = document.createElement("td");
        cell.textContent = cellValue;
        row.append(cell);
      }
      ui.leaderboardRows.append(row);
    }
  }

  async function buyUpgrade(upgrade) {
    const response = await fetch("/game/jaylive/shop", {
      method: "POST",
      headers: { "Accept": "application/json", "Content-Type": "application/json" },
      body: JSON.stringify({ upgrade }),
    });
    if (!response.ok) return;
    const data = await response.json();
    state.profile = data.profile;
    state.shop = data.shop || {};
    renderMeta();
  }

  async function setCharacter(action, character) {
    const response = await fetch("/game/jaylive/character", {
      method: "POST",
      headers: { "Accept": "application/json", "Content-Type": "application/json" },
      body: JSON.stringify({ action, character }),
    });
    if (!response.ok) return;
    const data = await response.json();
    state.profile = data.profile;
    state.shop = data.shop || {};
    renderMeta();
  }

  async function loadLeaderboard(sort = state.leaderboardSort) {
    state.leaderboardSort = sort;
    const response = await fetch(`/game/jaylive/leaderboard?sort=${encodeURIComponent(sort)}`, { headers: { Accept: "application/json" } });
    if (!response.ok) return;
    const data = await response.json();
    state.leaderboard = data.leaderboard || [];
    renderLeaderboard();
  }

  function leaderboardMeta(sort) {
    if (sort === "bestRunKills") {
      return {
        eyebrow: "Most Kills In One Run",
        label: "Run Kills",
        value: (entry) => entry.bestRunKills,
      };
    }
    if (sort === "bestRunSeconds") {
      return {
        eyebrow: "Longest Survival Run",
        label: "Run Time",
        value: (entry) => formatTime(entry.bestRunSeconds || 0),
      };
    }
    if (sort === "level") {
      return {
        eyebrow: "Highest Jaylive Level",
        label: "Level",
        value: (entry) => entry.level || 0,
      };
    }
    return {
      eyebrow: "Total Enemies Killed",
      label: "Total Kills",
      value: (entry) => entry.totalKills,
    };
  }

  function statsFromProfile() {
    const p = state.profile || {};
    return {
      maxHp: 100 + (p.maxHpLevel || 0) * 12,
      damage: 10 + (p.damageLevel || 0) * 2,
      speed: 245 + (p.moveSpeedLevel || 0) * 13,
      attackCooldown: attackCooldownFromLevel(p.attackSpeedLevel || 0),
    };
  }

  function attackCooldownFromLevel(level) {
    return Math.max(0.018, 0.62 / (1 + level * 0.045 + Math.sqrt(level) * 0.45));
  }

  async function startRun() {
    const runResponse = await fetch("/game/jaylive/start", {
      method: "POST",
      headers: { "Accept": "application/json" },
    });
    if (!runResponse.ok) return;
    const runData = await runResponse.json();
    state.runToken = runData.runToken || "";
    if (!state.runToken) return;

    const stats = statsFromProfile();
    player = {
      x: world.width / 2,
      y: world.height / 2,
      r: 24,
      angle: 0,
      hp: stats.maxHp,
      maxHp: stats.maxHp,
      baseMaxHp: stats.maxHp,
      baseDamage: stats.damage,
      baseSpeed: stats.speed,
      baseAttackCooldown: stats.attackCooldown,
      damage: stats.damage,
      speed: stats.speed,
      character: state.profile?.selectedCharacter || "jaylub",
      attackCooldown: stats.attackCooldown,
      attackTimer: 0,
      attackCount: 0,
      invuln: 0,
      kills: 0,
      runGold: 0,
      level: state.profile?.gameLevel || 0,
      xp: state.profile?.gameXP || 0,
      nextLevelXP: levelRequirement((state.profile?.gameLevel || 0) + 1),
      runDamageMult: 1,
      runSpeedMult: 1,
      runAttackSpeedMult: 1,
      runMaxHpBonus: 0,
      runItems: {
        aura: 0,
        football: 0,
        bike: 0,
        pigeon: 0,
      },
      itemTimers: {
        aura: 0,
        football: 0,
        bike: 5,
        pigeon: 5,
        footballAngle: 0,
      },
      survivalTime: 0,
      survivalSeconds: 0,
    };
    enemies = [];
    goldDrops = [];
    healDrops = [];
    particles = [];
    numbers = [];
    slashes = [];
    projectiles = [];
    bossProjectiles = [];
    treasureChests = [];
    itemChests = [];
    boostChests = [];
    boss = undefined;
    nextEnemyId = 1;
    state.spawnTimer = 0;
    state.healSpawnTimer = 8 + Math.random() * 10;
    state.spawnPauseTimer = 0;
    state.nextBossAt = 180;
    state.bossEncounter = 0;
    state.runToken = runData.runToken;
    state.itemChestTimer = 60;
    state.boostChestTimer = 30;
    state.shake = 0;
    state.hitPause = 0;
    state.paused = false;
    state.pausedAt = 0;
    state.dying = false;
    state.deathTimer = 0;
    state.submittedGameOver = false;
    state.running = true;
    runStatsDirty = true;
    cachedDifficulty = null;
    cachedDifficultyAt = -1;
    refreshRunStats();
    showScreen("game");
    updateHud();
    startGameLoop();
  }

  function markRunStatsDirty() {
    runStatsDirty = true;
  }

  function refreshRunStats() {
    if (!player) return;
    player.maxHp = player.baseMaxHp + player.runMaxHpBonus;
    player.damage = Math.max(1, Math.round(player.baseDamage * player.runDamageMult));
    player.speed = player.baseSpeed * player.runSpeedMult;
    player.attackCooldown = Math.max(0.014, (player.baseAttackCooldown * characterAttackDelay(player.character)) / player.runAttackSpeedMult);
    runStatsDirty = false;
  }

  function refreshRunStatsIfNeeded() {
    if (runStatsDirty) refreshRunStats();
  }

  function characterAttackDelay(character) {
    return character === "character3" ? 1.18 : 1;
  }

  function levelRequirement(level) {
    const step = Math.max(1, level);
    return 100 + (step - 1) * 70 + Math.floor(Math.pow(step - 1, 1.35) * 30);
  }

  function levelReward(level) {
    if (level === 1) {
      return {
        text: "+15% Damage +100 Gold",
        apply: (p) => {
          p.runDamageMult += 0.15;
          p.runGold += 100;
        },
      };
    }
    const rewards = [
      { text: "+7% Damage", apply: (p) => { p.runDamageMult += 0.07; } },
      { text: "+8% Max HP", apply: (p) => {
        const gain = Math.ceil(p.baseMaxHp * 0.08);
        p.runMaxHpBonus += gain;
        p.hp += gain;
      } },
      { text: "+4% Attack Speed", apply: (p) => { p.runAttackSpeedMult += 0.04; } },
      { text: "+5% Move Speed", apply: (p) => { p.runSpeedMult += 0.05; } },
      { text: "+75 Gold", apply: (p) => { p.runGold += 75; } },
    ];
    return rewards[(level - 2) % rewards.length];
  }

  function difficulty() {
    const seconds = player.survivalSeconds;
    if (cachedDifficultyAt === seconds) return cachedDifficulty;
    cachedDifficultyAt = seconds;
    const minutes = Math.max(0, seconds / 60);
    const sizeScale = Math.min(1.45, 1 + minutes * 0.035);
    cachedDifficulty = {
      hp: (26 + minutes * 14) * (1 + minutes * 0.025),
      damage: 8 + minutes * 3.2,
      speed: 86 + minutes * 7,
      spawnEvery: Math.max(0.22, 1.15 - minutes * 0.08),
      pack: Math.min(5, 1 + Math.floor(minutes / 1.5)),
      eliteChance: seconds < 45 ? 0 : Math.min(0.58, 0.08 + (seconds - 45) / 260),
      sizeScale,
    };
    return cachedDifficulty;
  }

  function spawnEnemy(options = {}) {
    const d = difficulty();
    const elite = options.elite ?? Math.random() < d.eliteChance;
    const side = Math.floor(Math.random() * 4);
    let x = 0;
    let y = 0;
    if (side === 0) { x = Math.random() * world.width; y = -40; }
    if (side === 1) { x = world.width + 40; y = Math.random() * world.height; }
    if (side === 2) { x = Math.random() * world.width; y = world.height + 40; }
    if (side === 3) { x = -40; y = Math.random() * world.height; }
    const sizeScale = options.sizeScale || d.sizeScale;
    const radius = (elite ? 28 : 22) * sizeScale;
    enemies.push({
      id: nextEnemyId++,
      x: options.x ?? x,
      y: options.y ?? y,
      type: elite ? "elite" : "normal",
      bossMinion: Boolean(options.bossMinion),
      r: radius,
      size: (elite ? 60 : 48) * sizeScale,
      hp: options.hp ?? (elite ? d.hp * 2.35 : d.hp),
      maxHp: options.hp ?? (elite ? d.hp * 2.35 : d.hp),
      damage: options.damage ?? (elite ? d.damage * 1.75 : d.damage),
      speed: options.speed ?? ((elite ? d.speed * 1.12 : d.speed) * (0.88 + Math.random() * 0.24)),
      hitTimer: 0,
      contactTimer: 0,
      footballContactTimer: 0,
      angle: 0,
      dead: false,
    });
  }

  function update(dt) {
    if (!state.running || !player) return;
    if (state.paused) return;
    if (state.dying) {
      state.deathTimer -= dt;
      state.shake = state.deathTimer > 0 ? 14 : 0;
      updateEffects(dt);
      updateHud();
      if (state.deathTimer <= 0) {
        endRun();
      }
      return;
    }
    if (state.hitPause > 0) {
      state.hitPause -= dt;
      return;
    }

    player.survivalTime += dt;
    player.survivalSeconds = Math.floor(player.survivalTime);
    player.attackTimer -= dt;
    player.invuln = Math.max(0, player.invuln - dt);
    state.shake = Math.max(0, state.shake - dt * 28);

    updateMouseWorld();
    refreshRunStatsIfNeeded();
    updatePlayer(dt);
    updateCamera();
    updateBossTiming(dt);
    updateBoss(dt);
    updateSpawns(dt);
    updateHealSpawns(dt);
    updateRunChestSpawns(dt);
    updateEnemies(dt);
    rebuildEnemyGrid();
    autoAttack();
    updateRunItems(dt);
    updateDrops(dt);
    updateProjectiles(dt);
    updateBossProjectiles(dt);
    updateEffects(dt);
    updateHud();

    if (player.hp <= 0) {
      startDeathShake();
    }
  }

  function updateMouseWorld() {
    mouse.worldX = mouse.x - viewport.left + camera.x;
    mouse.worldY = mouse.y - viewport.top + camera.y;
    player.angle = Math.atan2(mouse.worldY - player.y, mouse.worldX - player.x);
  }

  function updatePlayer(dt) {
    let ax = 0;
    let ay = 0;
    if (keys.has("KeyW")) ay -= 1;
    if (keys.has("KeyS")) ay += 1;
    if (keys.has("KeyA")) ax -= 1;
    if (keys.has("KeyD")) ax += 1;
    const len = Math.hypot(ax, ay) || 1;
    player.x += (ax / len) * player.speed * dt;
    player.y += (ay / len) * player.speed * dt;
    player.x = clamp(player.x, player.r, world.width - player.r);
    player.y = clamp(player.y, player.r, world.height - player.r);
  }

  function updateCamera() {
    camera.x = clamp(player.x - viewWidth() / 2, 0, world.width - viewWidth());
    camera.y = clamp(player.y - viewHeight() / 2, 0, world.height - viewHeight());
  }

  function updateBossTiming() {
    if (boss || state.spawnPauseTimer > 0) return;
    if (player.survivalSeconds >= state.nextBossAt) startBossEncounter();
  }

  function startBossEncounter() {
    state.bossEncounter++;
    const config = bossConfigs[Math.min(state.bossEncounter - 1, bossConfigs.length - 1)];
    enemies = [];
    bossProjectiles = [];
    state.spawnTimer = 0;
    state.healSpawnTimer = Math.max(state.healSpawnTimer, 4);

    const angle = Math.random() * Math.PI * 2;
    boss = {
      ...config,
      x: clamp(player.x + Math.cos(angle) * 520, config.r + 30, world.width - config.r - 30),
      y: clamp(player.y + Math.sin(angle) * 520, config.r + 30, world.height - config.r - 30),
      hp: config.hp,
      maxHp: config.hp,
      attackTimer: 1.2,
      patternIndex: 0,
      minionTimer: 0,
      elapsed: 0,
      hitTimer: 0,
      footballContactTimer: 0,
      angle: 0,
    };
    pushNumber({ x: boss.x, y: boss.y - boss.r - 28, text: `${boss.name} appears`, color: "#d5a43a", life: 1.4 });
    burst(boss.x, boss.y, "#d5a43a", 42);
    maintainBossMinions();
  }

  function updateBoss(dt) {
    if (!boss) return;

    boss.elapsed += dt;
    const dx = player.x - boss.x;
    const dy = player.y - boss.y;
    const dist = Math.hypot(dx, dy) || 1;
    boss.angle = Math.atan2(dy, dx);
    const desired = 340;
    const moveSign = dist > desired + 60 ? 1 : dist < desired - 90 ? -1 : 0;
    boss.x = clamp(boss.x + (dx / dist) * boss.speed * moveSign * dt, boss.r, world.width - boss.r);
    boss.y = clamp(boss.y + (dy / dist) * boss.speed * moveSign * dt, boss.r, world.height - boss.r);
    boss.hitTimer = Math.max(0, boss.hitTimer - dt);
    boss.footballContactTimer = Math.max(0, boss.footballContactTimer - dt);

    boss.attackTimer -= dt;
    if (boss.attackTimer <= 0) {
      performBossAttack();
      boss.patternIndex = (boss.patternIndex + 1) % 3;
      boss.attackTimer = boss.cooldown;
    }

    boss.minionTimer -= dt;
    if (boss.minionTimer <= 0) {
      maintainBossMinions();
      boss.minionTimer = 15;
    }
  }

  function performBossAttack() {
    if (!boss) return;
    if (boss.patternIndex === 0) {
      fireBossProjectile(boss.angle);
      return;
    }
    if (boss.patternIndex === 1) {
      const spread = Math.PI / 9;
      for (let i = -2; i <= 2; i++) fireBossProjectile(boss.angle + i * spread);
      return;
    }
    for (let i = 0; i < boss.burstShots; i++) {
      fireBossProjectile((Math.PI * 2 * i) / boss.burstShots);
    }
  }

  function fireBossProjectile(angle) {
    const speed = boss.shotSpeed;
    bossProjectiles.push({
      x: boss.x + Math.cos(angle) * (boss.r + 12),
      y: boss.y + Math.sin(angle) * (boss.r + 12),
      vx: Math.cos(angle) * speed,
      vy: Math.sin(angle) * speed,
      r: 17,
      damage: boss.damage,
      angle,
      life: 6,
    });
  }

  function updateBossProjectiles(dt) {
    for (let i = bossProjectiles.length - 1; i >= 0; i--) {
      const projectile = bossProjectiles[i];
      projectile.x += projectile.vx * dt;
      projectile.y += projectile.vy * dt;
      projectile.life -= dt;

      if (circleHit(player, projectile) && player.invuln <= 0) {
        damagePlayer(projectile.damage);
        burst(projectile.x, projectile.y, "#a93b36", 10);
        bossProjectiles.splice(i, 1);
        continue;
      }

      if (
        projectile.life <= 0 ||
        projectile.x < -80 ||
        projectile.y < -80 ||
        projectile.x > world.width + 80 ||
        projectile.y > world.height + 80
      ) {
        bossProjectiles.splice(i, 1);
      }
    }
  }

  function maintainBossMinions() {
    let active = 0;
    for (const enemy of enemies) {
      if (enemy.bossMinion) active++;
    }
    for (let i = active; i < 5; i++) spawnBossMinion();
  }

  function spawnBossMinion() {
    const angle = Math.random() * Math.PI * 2;
    const distance = 260 + Math.random() * 180;
    const d = difficulty();
    spawnEnemy({
      x: clamp((boss?.x ?? player.x) + Math.cos(angle) * distance, 80, world.width - 80),
      y: clamp((boss?.y ?? player.y) + Math.sin(angle) * distance, 80, world.height - 80),
      elite: true,
      bossMinion: true,
      sizeScale: Math.max(1.05, d.sizeScale),
      hp: d.hp * 2.1,
      damage: d.damage * 1.55,
      speed: d.speed * 1.08,
    });
  }

  function damageBoss(amount, x, y) {
    if (!boss) return;
    boss.hp -= amount;
    boss.hitTimer = 0.14;
    pushNumber({ x: x || boss.x, y: (y || boss.y) - boss.r - 18, text: String(amount), color: "#f1ead7", life: 0.66 });
    burst(x || boss.x, y || boss.y, "#d5a43a", 10);
    if (boss.hp <= 0) defeatBoss();
  }

  function defeatBoss() {
    if (!boss) return;
    const defeated = boss;
    treasureChests.push({
      x: defeated.x,
      y: defeated.y,
      value: defeated.reward,
      life: 0,
      collected: false,
    });
    pushNumber({ x: defeated.x, y: defeated.y - defeated.r - 36, text: `${defeated.name} defeated`, color: "#d5a43a", life: 1.35 });
    burst(defeated.x, defeated.y, "#d5a43a", 60);
    boss = undefined;
    bossProjectiles = [];
    enemies = [];
    state.spawnPauseTimer = 5;
    state.nextBossAt = player.survivalSeconds + 180;
  }

  function updateSpawns(dt) {
    if (boss) {
      if (boss.elapsed < bossSpawnGraceSeconds) return;
    } else {
      state.spawnPauseTimer = Math.max(0, state.spawnPauseTimer - dt);
      if (state.spawnPauseTimer > 0) return;
    }

    const d = difficulty();
    state.spawnTimer -= dt;
    if (state.spawnTimer <= 0) {
      for (let i = 0; i < d.pack; i++) spawnEnemy();
      state.spawnTimer = d.spawnEvery;
    }
  }

  function updateHealSpawns(dt) {
    state.healSpawnTimer -= dt;
    if (state.healSpawnTimer > 0) return;

    const missingHp = player.maxHp - player.hp;
    const healChance = missingHp > 0 ? 0.46 : 0.16;
    if (healDrops.length < 3 && Math.random() < healChance) {
      healDrops.push({
        x: 80 + Math.random() * (world.width - 160),
        y: 80 + Math.random() * (world.height - 160),
        amount: 15,
        life: 0,
      });
    }
    state.healSpawnTimer = 9 + Math.random() * 12;
  }

  function updateRunChestSpawns(dt) {
    state.itemChestTimer -= dt;
    if (state.itemChestTimer <= 0) {
      spawnRunChest("item");
      state.itemChestTimer = 60;
    }

    state.boostChestTimer -= dt;
    if (state.boostChestTimer <= 0) {
      spawnRunChest("boost");
      state.boostChestTimer = 30;
    }
  }

  function spawnRunChest(type) {
    const spot = randomMapSpot(96);
    const chest = {
      x: spot.x,
      y: spot.y,
      type,
      life: 0,
      collected: false,
    };
    if (type === "item") {
      itemChests.push(chest);
    } else {
      boostChests.push(chest);
    }
    pushNumber({
      x: chest.x,
      y: chest.y - 28,
      text: type === "item" ? "Item chest" : "Boost chest",
      color: type === "item" ? "#d5a43a" : "#9bd27b",
      life: 1.2,
    });
  }

  function randomMapSpot(margin) {
    return {
      x: margin + Math.random() * (world.width - margin * 2),
      y: margin + Math.random() * (world.height - margin * 2),
    };
  }

  function updateEnemies(dt) {
    for (const enemy of enemies) {
      const dx = player.x - enemy.x;
      const dy = player.y - enemy.y;
      const len = Math.hypot(dx, dy) || 1;
      enemy.angle = Math.atan2(dy, dx);
      enemy.x += (dx / len) * enemy.speed * dt;
      enemy.y += (dy / len) * enemy.speed * dt;
      enemy.hitTimer = Math.max(0, enemy.hitTimer - dt);
      enemy.contactTimer = Math.max(0, enemy.contactTimer - dt);
      enemy.footballContactTimer = Math.max(0, enemy.footballContactTimer - dt);

      if (circleHit(player, enemy) && enemy.contactTimer <= 0 && player.invuln <= 0) {
        damagePlayer(enemy.damage);
        enemy.contactTimer = 0.65;
      }
    }
  }

  function updateRunItems(dt) {
    if (!player) return;

    if (player.runItems.aura > 0) {
      player.itemTimers.aura -= dt;
      if (player.itemTimers.aura <= 0) {
        damageEnemiesInRadius(player.x, player.y, 170, abilityDamage("aura", 0.1 * player.runItems.aura), "#7fb069");
        player.itemTimers.aura = 0.35;
      }
    }

    if (player.runItems.football > 0) {
      player.itemTimers.footballAngle += dt * (2.8 + player.runItems.football * 0.18);
      for (let i = 0; i < player.runItems.football; i++) {
        damageFootballTouches(footballPosition(i));
      }
    }

    if (player.runItems.bike > 0) {
      player.itemTimers.bike -= dt;
      if (player.itemTimers.bike <= 0) {
        for (let i = 0; i < player.runItems.bike; i++) {
          fireItemProjectile("bike", player.angle + (i - (player.runItems.bike - 1) / 2) * 0.14, abilityDamage("bike", 0.35), 620);
        }
        player.itemTimers.bike = 5;
      }
    }

    if (player.runItems.pigeon > 0) {
      player.itemTimers.pigeon -= dt;
      if (player.itemTimers.pigeon <= 0) {
        const spread = 35 * Math.PI / 180;
        for (let stack = 0; stack < player.runItems.pigeon; stack++) {
          const offset = (stack - (player.runItems.pigeon - 1) / 2) * 0.08;
          for (const angle of [player.angle + offset, player.angle + spread + offset, player.angle - spread + offset]) {
            fireItemProjectile("pigeon", angle, abilityDamage("pigeon", 0.75), 720);
          }
        }
        player.itemTimers.pigeon = 5;
      }
    }
  }

  function footballPosition(index) {
    const count = Math.max(1, player.runItems.football);
    const angle = player.itemTimers.footballAngle + (Math.PI * 2 * index) / count;
    const radius = 102 + (index % 2) * 18;
    const bounce = Math.sin(player.itemTimers.footballAngle * 3 + index) * 12;
    return {
      x: player.x + Math.cos(angle) * radius,
      y: player.y + Math.sin(angle) * (radius + bounce),
    };
  }

  function damageFootballTouches(ball) {
    const radius = 52;
    const damage = abilityDamage("football", 0.3);
    forEachNearbyEnemy(ball.x, ball.y, radius + 36, (enemy) => {
      if (enemy.footballContactTimer > 0) return;
      if (!circlesOverlap(enemy.x, enemy.y, enemy.r, ball.x, ball.y, radius)) return;
      enemy.footballContactTimer = 0.32;
      damageEnemy(enemy, damage, ball.x, ball.y, "#f1ead7");
    });
    if (boss && boss.footballContactTimer <= 0 && circlesOverlap(boss.x, boss.y, boss.r, ball.x, ball.y, radius)) {
      boss.footballContactTimer = 0.32;
      damageBoss(damage, ball.x, ball.y);
    }
  }

  function abilityDamage(kind, ratio) {
    const levels = {
      aura: state.profile?.auraDamageLevel || 0,
      football: state.profile?.footballDamageLevel || 0,
      bike: state.profile?.bikeDamageLevel || 0,
      pigeon: state.profile?.pigeonDamageLevel || 0,
    };
    const level = levels[kind] || 0;
    return Math.max(1, Math.ceil(player.damage * ratio * (1 + level * 0.12)));
  }

  function fireItemProjectile(kind, angle, damage, speed) {
    projectiles.push({
      x: player.x + Math.cos(angle) * 34,
      y: player.y + Math.sin(angle) * 34,
      vx: Math.cos(angle) * speed,
      vy: Math.sin(angle) * speed,
      r: kind === "pigeon" ? 24 : 28,
      damage,
      pierceLeft: kind === "pigeon" ? 1 : 0,
      hitEnemyIds: new Set(),
      life: kind === "pigeon" ? 1.25 : 1.1,
      angle,
      kind,
    });
  }

  function damageEnemiesInRadius(x, y, radius, amount, color) {
    forEachNearbyEnemy(x, y, radius + 40, (enemy) => {
      if (!circlesOverlap(enemy.x, enemy.y, enemy.r, x, y, radius)) return;
      damageEnemy(enemy, amount, enemy.x, enemy.y, color);
    });
    if (boss && circlesOverlap(boss.x, boss.y, boss.r, x, y, radius)) {
      damageBoss(amount, x, y);
    }
  }

  function damageEnemy(enemy, amount, x, y, color) {
    if (!enemy || enemy.dead) return;
    enemy.hp -= amount;
    enemy.hitTimer = 0.12;
    pushNumber({ x: x || enemy.x, y: (y || enemy.y) - 28, text: String(amount), color, life: 0.62 });
    burst(x || enemy.x, y || enemy.y, color, 7);
    if (enemy.hp <= 0) killEnemyByRef(enemy);
  }

  function damageEnemyAt(index, amount, x, y, color) {
    damageEnemy(enemies[index], amount, x, y, color);
  }

  function updateDrops(dt) {
    updateRunChestDrops(itemChests, openItemChest, dt);
    updateRunChestDrops(boostChests, openBoostChest, dt);

    for (let i = treasureChests.length - 1; i >= 0; i--) {
      const chest = treasureChests[i];
      chest.life += dt;
      if (!chest.collected && circlesOverlap(player.x, player.y, player.r, chest.x, chest.y, 34)) {
        chest.collected = true;
        const healed = Math.ceil(player.maxHp * 0.5);
        player.runGold += chest.value;
        player.hp += healed;
        pushNumber({ x: chest.x, y: chest.y - 34, text: `+${chest.value} gold`, color: "#d5a43a", life: 1.1 });
        pushNumber({ x: chest.x, y: chest.y - 58, text: `+${healed} HP`, color: "#9bd27b", life: 1.1 });
        burst(chest.x, chest.y, "#d5a43a", 36);
        burst(chest.x, chest.y, "#7fb069", 18);
        treasureChests.splice(i, 1);
      }
    }

    for (let i = goldDrops.length - 1; i >= 0; i--) {
      const drop = goldDrops[i];
      drop.life += dt;
      if (circlesOverlap(player.x, player.y, player.r, drop.x, drop.y, 18)) {
        player.runGold += drop.value;
        burst(drop.x, drop.y, "#d5a43a", 10);
        pushNumber({ x: drop.x, y: drop.y - 20, text: `+${drop.value}`, color: "#d5a43a", life: 0.8 });
        goldDrops.splice(i, 1);
      }
    }

    for (let i = healDrops.length - 1; i >= 0; i--) {
      const drop = healDrops[i];
      drop.life += dt;
      if (circlesOverlap(player.x, player.y, player.r, drop.x, drop.y, 20)) {
        const healed = drop.amount;
        player.hp += healed;
        burst(drop.x, drop.y, "#7fb069", 14);
        pushNumber({ x: drop.x, y: drop.y - 20, text: `+${healed} HP`, color: "#9bd27b", life: 0.9 });
        healDrops.splice(i, 1);
      }
    }
  }

  function updateRunChestDrops(chests, opener, dt) {
    for (let i = chests.length - 1; i >= 0; i--) {
      const chest = chests[i];
      chest.life += dt;
      if (!chest.collected && circlesOverlap(player.x, player.y, player.r, chest.x, chest.y, 34)) {
        chest.collected = true;
        opener(chest);
        chests.splice(i, 1);
      }
    }
  }

  function openItemChest(chest) {
    const missing = itemPool.filter((entry) => (player.runItems[entry.id] || 0) === 0);
    const item = weightedPick(missing.length ? missing : itemPool);
    if (!missing.length) {
      const gold = duplicateItemGold(item.rarity);
      player.runGold += gold;
      pushNumber({ x: chest.x, y: chest.y - 34, text: `${item.rarity}: +${gold} gold`, color: item.color, life: 1.35 });
      burst(chest.x, chest.y, item.color, item.rarity === "Legendary" ? 50 : 30);
      return;
    }
    player.runItems[item.id]++;
    if (item.id === "football") player.itemTimers.football = 0;
    if (item.id === "bike") player.itemTimers.bike = Math.min(player.itemTimers.bike, 0.35);
    if (item.id === "pigeon") player.itemTimers.pigeon = Math.min(player.itemTimers.pigeon, 0.35);
    pushNumber({ x: chest.x, y: chest.y - 34, text: `${item.rarity}: ${item.name}`, color: item.color, life: 1.35 });
    burst(chest.x, chest.y, item.color, item.rarity === "Legendary" ? 50 : 30);
  }

  function duplicateItemGold(rarity) {
    if (rarity === "Rare") return 50;
    if (rarity === "Epic") return 75;
    if (rarity === "Legendary") return 100;
    return 25;
  }

  function openBoostChest(chest) {
    const boost = boostPool[Math.floor(Math.random() * boostPool.length)];
    boost.apply(player);
    markRunStatsDirty();
    refreshRunStatsIfNeeded();
    pushNumber({ x: chest.x, y: chest.y - 34, text: boost.name, color: boost.color, life: 1.2 });
    burst(chest.x, chest.y, boost.color, 26);
  }

  function weightedPick(pool) {
    const total = pool.reduce((sum, entry) => sum + entry.weight, 0);
    let roll = Math.random() * total;
    for (const entry of pool) {
      roll -= entry.weight;
      if (roll <= 0) return entry;
    }
    return pool[pool.length - 1];
  }

  function updateEffects(dt) {
    for (let i = particles.length - 1; i >= 0; i--) {
      const p = particles[i];
      p.x += p.vx * dt;
      p.y += p.vy * dt;
      p.life -= dt;
      if (p.life <= 0) particles.splice(i, 1);
    }
    for (let i = numbers.length - 1; i >= 0; i--) {
      const n = numbers[i];
      n.y -= 36 * dt;
      n.life -= dt;
      if (n.life <= 0) numbers.splice(i, 1);
    }
    for (let i = slashes.length - 1; i >= 0; i--) {
      slashes[i].life -= dt;
      if (slashes[i].life <= 0) slashes.splice(i, 1);
    }
  }

  function updateProjectiles(dt) {
    for (let i = projectiles.length - 1; i >= 0; i--) {
      const projectile = projectiles[i];
      projectile.x += projectile.vx * dt;
      projectile.y += projectile.vy * dt;
      projectile.life -= dt;

      let hit = false;
      forEachNearbyEnemy(projectile.x, projectile.y, projectile.r + 48, (enemy) => {
        if (hit || projectile.hitEnemyIds.has(enemy.id)) return;
        if (!circleHit(projectile, enemy)) return;

        projectile.hitEnemyIds.add(enemy.id);
        damageEnemy(enemy, projectile.damage, projectile.x, projectile.y, projectileColor(projectile));
        projectile.pierceLeft--;
        if (projectile.pierceLeft < 0) {
          hit = true;
        }
      });

      if (!hit && boss && !projectile.hitBoss && circleHit(projectile, boss)) {
        projectile.hitBoss = true;
        damageBoss(projectile.damage, projectile.x, projectile.y);
        hit = true;
      }

      if (hit || projectile.life <= 0 || projectile.x < 0 || projectile.y < 0 || projectile.x > world.width || projectile.y > world.height) {
        projectiles.splice(i, 1);
      }
    }
  }

  function attack() {
    if (!state.running || state.paused || state.dying || player.attackTimer > 0) return false;
    if (player.character === "goblin_jaylub") {
      shootProjectile();
      return true;
    }
    if (player.character === "character3") {
      shootVampireKnife();
      return true;
    }

    player.attackCount++;
    const strong = player.attackCount % 3 === 0;
    const reach = strong ? 190 : 105;
    const width = strong ? 1.02 : 0.82;
    const damage = strong ? Math.ceil(player.damage * 1.45) : player.damage;
    const slash = { x: player.x, y: player.y, angle: player.angle, life: 0.16, maxLife: 0.16, reach, width, strong };
    slashes.push(slash);
    state.shake = Math.max(state.shake, strong ? 5 : 3);

    forEachNearbyEnemy(player.x, player.y, reach + 50, (enemy) => {
      const dx = enemy.x - player.x;
      const dy = enemy.y - player.y;
      const maxReach = reach + enemy.r;
      if (distanceSq(dx, dy) >= maxReach * maxReach) return;
      const angle = Math.atan2(dy, dx);
      const diff = Math.abs(wrapAngle(angle - player.angle));
      if (diff < width) {
        damageEnemy(enemy, damage, enemy.x, enemy.y, strong ? "#d5a43a" : "#f1ead7");
      }
    });

    if (boss) {
      const dx = boss.x - player.x;
      const dy = boss.y - player.y;
      const maxReach = boss.r + reach;
      if (distanceSq(dx, dy) < maxReach * maxReach) {
        const angle = Math.atan2(dy, dx);
        const diff = Math.abs(wrapAngle(angle - player.angle));
        if (diff < width + 0.1) {
          damageBoss(damage, boss.x, boss.y);
        }
      }
    }
    return true;
  }

  function autoAttack() {
    let attacks = 0;
    while (attacks < 4 && attack()) {
      player.attackTimer += player.attackCooldown;
      attacks++;
    }
    if (attacks >= 4 && player.attackTimer <= 0) {
      player.attackTimer = player.attackCooldown;
    }
  }

  function shootProjectile() {
    const speed = 680;
    const vx = Math.cos(player.angle) * speed;
    const vy = Math.sin(player.angle) * speed;
    projectiles.push({
      x: player.x + Math.cos(player.angle) * 32,
      y: player.y + Math.sin(player.angle) * 32,
      vx,
      vy,
      r: 8,
      damage: player.damage,
      pierceLeft: state.profile?.piercingLevel || 0,
      hitEnemyIds: new Set(),
      life: 1.15,
      angle: player.angle,
      kind: "goblin",
    });
    state.shake = Math.max(state.shake, 2);
  }

  function shootVampireKnife() {
    const speed = 760;
    const damage = Math.max(1, Math.ceil(player.damage * 0.42));
    const spread = Math.PI / 4;
    // Vampire Jaylub fires a fixed aim-based spread: center, 45 degrees left, 45 degrees right.
    for (const angle of [player.angle, player.angle - spread, player.angle + spread]) {
      projectiles.push({
        x: player.x + Math.cos(angle) * 34,
        y: player.y + Math.sin(angle) * 34,
        vx: Math.cos(angle) * speed,
        vy: Math.sin(angle) * speed,
        r: 9,
        damage,
        pierceLeft: 0,
        hitEnemyIds: new Set(),
        life: 0.95,
        angle,
        kind: "vampireKnife",
      });
    }
    state.shake = Math.max(state.shake, 2);
  }

  function projectileColor(projectile) {
    if (projectile.kind === "bike") return "#5992a7";
    if (projectile.kind === "pigeon") return "#d5a43a";
    if (projectile.kind === "vampireKnife") return "#d95b4d";
    return "#7fb069";
  }

  function killEnemy(index) {
    const enemy = enemies[index];
    if (!enemy || enemy.dead) return;
    enemy.dead = true;
    player.kills++;
    gainXP(1);
    if (player.character === "character3") {
      // Vampire Jaylub lifesteal is tied to confirmed kills here to avoid duplicate healing.
      const healed = 1 + Math.floor(Math.random() * 2);
      player.hp += healed;
      pushNumber({ x: player.x, y: player.y - 46, text: `+${healed} HP`, color: "#9bd27b", life: 0.72 });
      burst(player.x, player.y, "#9bd27b", 6);
    }
    const goldChance = enemy.type === "elite" ? 0.82 : 0.58;
    if (Math.random() < goldChance) {
      const value = enemy.type === "elite" ? 4 + Math.floor(Math.random() * 5) : 1 + Math.floor(Math.random() * 4);
      goldDrops.push({ x: enemy.x, y: enemy.y, value, life: 0 });
    }
    burst(enemy.x, enemy.y, enemy.type === "elite" ? "#9f5f3a" : "#6f7356", enemy.type === "elite" ? 20 : 12);
    const last = enemies.length - 1;
    if (index !== last) enemies[index] = enemies[last];
    enemies.pop();
  }

  function killEnemyByRef(enemy) {
    const index = enemies.indexOf(enemy);
    if (index !== -1) killEnemy(index);
  }

  function gainXP(amount) {
    player.xp += amount;
    while (player.xp >= player.nextLevelXP) {
      player.xp -= player.nextLevelXP;
      player.level++;
      const reward = levelReward(player.level);
      reward.apply(player);
      markRunStatsDirty();
      refreshRunStatsIfNeeded();
      player.nextLevelXP = levelRequirement(player.level + 1);
      pushNumber({ x: player.x, y: player.y - 58, text: `Level ${player.level}: ${reward.text}`, color: "#d5a43a", life: 1.4 });
      burst(player.x, player.y, "#d5a43a", 32);
    }
  }

  function damagePlayer(amount) {
    if (state.dying) return;
    player.hp -= Math.ceil(amount);
    player.invuln = 0.45;
    state.hitPause = 0.035;
    state.shake = 12;
    pushNumber({ x: player.x, y: player.y - 34, text: `-${Math.ceil(amount)}`, color: "#d95b4d", life: 0.75 });
    burst(player.x, player.y, "#d95b4d", 12);
  }

  function startDeathShake() {
    if (state.dying) return;
    player.hp = 0;
    state.dying = true;
    state.deathTimer = 1;
    state.hitPause = 0;
    state.shake = 14;
    burst(player.x, player.y, "#d95b4d", 24);
  }

  function pauseRun() {
    if (!state.running || state.paused || state.dying) return;
    state.paused = true;
    state.pausedAt = performance.now();
    keys.clear();
    showScreen("pause");
    draw();
    stopGameLoop();
  }

  function resumeRun() {
    if (!state.running || !state.paused || !player) return;
    state.paused = false;
    state.pausedAt = 0;
    state.lastTime = performance.now();
    showScreen("game");
    startGameLoop();
  }

  function quitRun() {
    stopGameLoop();
    state.running = false;
    state.paused = false;
    state.pausedAt = 0;
    state.dying = false;
    state.deathTimer = 0;
    state.shake = 0;
    keys.clear();
    player = undefined;
    enemies = [];
    goldDrops = [];
    healDrops = [];
    particles = [];
    numbers = [];
    slashes = [];
    projectiles = [];
    bossProjectiles = [];
    treasureChests = [];
    itemChests = [];
    boostChests = [];
    boss = undefined;
    state.spawnPauseTimer = 0;
    state.runToken = "";
    state.itemChestTimer = 60;
    state.boostChestTimer = 30;
    showScreen("menu");
  }

  async function endRun() {
    stopGameLoop();
    state.running = false;
    ui.finalTime.textContent = formatTime(player.survivalSeconds);
    ui.finalKills.textContent = player.kills;
    ui.finalGold.textContent = player.runGold;
    showScreen("gameOver");

    if (state.submittedGameOver) return;
    state.submittedGameOver = true;
    const response = await fetch("/game/jaylive/run", {
      method: "POST",
      headers: { "Accept": "application/json", "Content-Type": "application/json" },
      body: JSON.stringify({
        kills: player.kills,
        gold: player.runGold,
        survivalSeconds: player.survivalSeconds,
        runToken: state.runToken,
      }),
    });
    if (response.ok) {
      const data = await response.json();
      state.profile = data.profile;
      state.shop = data.shop || state.shop;
      state.leaderboard = data.leaderboard || state.leaderboard;
      renderMeta();
    }
  }

  function draw() {
    const w = viewWidth();
    const h = viewHeight();
    ctx.clearRect(0, 0, w, h);
    ctx.save();
    const shakeX = state.shake ? (Math.random() - 0.5) * state.shake : 0;
    const shakeY = state.shake ? (Math.random() - 0.5) * state.shake : 0;
    ctx.translate(Math.round(-camera.x + shakeX), Math.round(-camera.y + shakeY));

    drawWorld();
    if (player) drawAura();
    for (const drop of goldDrops) if (isCircleVisible(drop.x, drop.y, 24)) drawGold(drop);
    for (const drop of healDrops) if (isCircleVisible(drop.x, drop.y, 28)) drawHeal(drop);
    for (const chest of treasureChests) if (isCircleVisible(chest.x, chest.y, 38)) drawTreasure(chest);
    for (const chest of itemChests) if (isCircleVisible(chest.x, chest.y, 36)) drawRunChest(chest);
    for (const chest of boostChests) if (isCircleVisible(chest.x, chest.y, 36)) drawRunChest(chest);
    for (const enemy of enemies) if (isCircleVisible(enemy.x, enemy.y, enemy.size || enemy.r * 2)) drawEnemy(enemy);
    if (boss && isCircleVisible(boss.x, boss.y, boss.size)) drawBoss();
    for (const projectile of bossProjectiles) if (isCircleVisible(projectile.x, projectile.y, 32)) drawBossProjectile(projectile);
    if (player) {
      drawPlayer();
      drawFootball();
      for (const slash of slashes) if (isCircleVisible(slash.x, slash.y, slash.reach || 105)) drawSlash(slash);
      for (const projectile of projectiles) if (isCircleVisible(projectile.x, projectile.y, 42)) drawProjectile(projectile);
    }
    for (const p of particles) if (isCircleVisible(p.x, p.y, p.size || 4, 24)) drawParticle(p);
    drawNumbers();

    ctx.restore();
  }

  function drawWorld() {
    const sx = Math.floor(camera.x);
    const sy = Math.floor(camera.y);
    const sw = Math.min(worldCanvas.width - sx, Math.ceil(viewWidth()) + 2);
    const sh = Math.min(worldCanvas.height - sy, Math.ceil(viewHeight()) + 2);
    ctx.drawImage(worldCanvas, sx, sy, sw, sh, sx, sy, sw, sh);
  }

  function drawPlayer() {
    const image = playerImageForCharacter(player.character);
    ctx.save();
    ctx.translate(Math.round(player.x), Math.round(player.y));
    ctx.rotate(player.angle);
    ctx.fillStyle = "rgba(0, 0, 0, 0.28)";
    ctx.fillRect(-22, 18, 44, 10);
    if (image.complete && image.naturalWidth) {
      ctx.drawImage(image, -28, -28, 56, 56);
    } else {
      ctx.fillStyle = player.character === "goblin_jaylub" ? "#7fb069" : player.character === "character3" ? "#d95b4d" : "#d5a43a";
      ctx.fillRect(-24, -24, 48, 48);
    }
    ctx.fillStyle = "#e7dfc9";
    ctx.fillRect(18, -4, 18, 8);
    ctx.restore();
  }

  function playerImageForCharacter(character) {
    if (character === "goblin_jaylub") return goblinImage;
    if (character === "character3") return character3Image;
    return playerImage;
  }

  function drawEnemy(enemy) {
    const size = enemy.size || (enemy.type === "elite" ? 60 : 48);
    const half = size / 2;
    const shadowW = enemy.type === "elite" ? size * 0.56 : size * 0.82;
    const shadowH = enemy.type === "elite" ? 7 : 10;
    const image = enemy.type === "elite" ? enemy2Image : enemyImage;
    ctx.save();
    ctx.translate(Math.round(enemy.x), Math.round(enemy.y));
    ctx.rotate(enemy.angle);
    ctx.globalAlpha = enemy.hitTimer > 0 ? 0.65 : 1;
    ctx.fillStyle = "rgba(0, 0, 0, 0.28)";
    ctx.fillRect(-shadowW / 2, half - 4, shadowW, shadowH);
    if (image.complete && image.naturalWidth) {
      ctx.drawImage(image, -half, -half, size, size);
    } else {
      ctx.fillStyle = enemy.type === "elite" ? "#7a4c3a" : "#758d54";
      ctx.fillRect(-half + 2, -half + 2, size - 4, size - 4);
    }
    ctx.restore();

    const barW = enemy.type === "elite" ? 48 : 38;
    ctx.fillStyle = "#0a0d0e";
    ctx.fillRect(enemy.x - barW / 2, enemy.y - half - 10, barW, 6);
    ctx.fillStyle = enemy.type === "elite" ? "#c46a42" : "#a93b36";
    ctx.fillRect(enemy.x - barW / 2 + 2, enemy.y - half - 8, (barW - 4) * clamp(enemy.hp / enemy.maxHp, 0, 1), 2);
  }

  function drawBoss() {
    const image = bossImages[boss.imageIndex];
    const half = boss.size / 2;
    ctx.save();
    ctx.translate(Math.round(boss.x), Math.round(boss.y));
    ctx.rotate(boss.angle);
    ctx.globalAlpha = boss.hitTimer > 0 ? 0.72 : 1;
    ctx.fillStyle = "rgba(0, 0, 0, 0.34)";
    ctx.fillRect(-half * 0.55, half - 8, half * 1.1, 16);
    if (image.complete && image.naturalWidth) {
      ctx.drawImage(image, -half, -half, boss.size, boss.size);
    } else {
      ctx.fillStyle = "#8a4f3b";
      ctx.fillRect(-half, -half, boss.size, boss.size);
    }
    ctx.restore();
  }

  function drawSlash(slash) {
    const t = slash.life / slash.maxLife;
    const reach = slash.reach || 76;
    const bladeW = slash.strong ? 18 : 12;
    const bladeH = slash.strong ? 88 : 68;
    ctx.save();
    ctx.translate(slash.x, slash.y);
    ctx.rotate(slash.angle);
    ctx.globalAlpha = Math.max(0, t);
    ctx.fillStyle = slash.strong ? "#d5a43a" : "#e7dfc9";
    ctx.fillRect(24, -bladeH / 2, bladeW, bladeH);
    ctx.fillStyle = slash.strong ? "#f1d879" : "#d5a43a";
    ctx.fillRect(36, -bladeH / 2 + 8, bladeW, bladeH - 16);
    ctx.fillStyle = "#f1ead7";
    ctx.fillRect(reach - 28, -14, 28, 28);
    ctx.restore();
  }

  function drawProjectile(projectile) {
    ctx.save();
    ctx.translate(projectile.x, projectile.y);
    ctx.rotate(projectile.angle);
    if (projectile.kind === "bike") {
      drawItemImage(bikeImage, 62, 46, "#5992a7");
    } else if (projectile.kind === "pigeon") {
      drawItemImage(pigeonImage, 52, 44, "#d5a43a");
    } else if (projectile.kind === "vampireKnife") {
      ctx.fillStyle = "#0a0d0e";
      ctx.fillRect(-16, -5, 28, 10);
      ctx.fillStyle = "#d95b4d";
      ctx.fillRect(-12, -3, 20, 6);
      ctx.fillStyle = "#f1ead7";
      ctx.fillRect(6, -2, 12, 4);
    } else {
      ctx.fillStyle = "#0a0d0e";
      ctx.fillRect(-8, -8, 20, 16);
      ctx.fillStyle = "#7fb069";
      ctx.fillRect(-5, -5, 16, 10);
      ctx.fillStyle = "#c8e6a0";
      ctx.fillRect(4, -3, 5, 4);
    }
    ctx.restore();
  }

  function drawBossProjectile(projectile) {
    const size = 34;
    ctx.save();
    ctx.translate(projectile.x, projectile.y);
    ctx.rotate(projectile.angle);
    ctx.fillStyle = "rgba(0, 0, 0, 0.3)";
    ctx.fillRect(-13, 12, 26, 6);
    if (enemyImage.complete && enemyImage.naturalWidth) {
      ctx.drawImage(enemyImage, -size / 2, -size / 2, size, size);
    } else {
      ctx.fillStyle = "#758d54";
      ctx.fillRect(-14, -14, 28, 28);
    }
    ctx.restore();
  }

  function drawGold(drop) {
    const bob = Math.sin(drop.life * 8) * 3;
    ctx.fillStyle = "#0a0d0e";
    ctx.fillRect(drop.x - 8, drop.y + 8, 16, 5);
    ctx.fillStyle = "#d5a43a";
    ctx.fillRect(drop.x - 7, drop.y - 7 + bob, 14, 14);
    ctx.fillStyle = "#f1d879";
    ctx.fillRect(drop.x - 3, drop.y - 5 + bob, 5, 4);
  }

  function drawHeal(drop) {
    const bob = Math.sin(drop.life * 6) * 4;
    ctx.fillStyle = "rgba(0, 0, 0, 0.28)";
    ctx.fillRect(drop.x - 14, drop.y + 13, 28, 7);
    if (healImage.complete && healImage.naturalWidth) {
      ctx.drawImage(healImage, drop.x - 18, drop.y - 18 + bob, 36, 36);
      return;
    }
    ctx.fillStyle = "#e7dfc9";
    ctx.fillRect(drop.x - 14, drop.y - 14 + bob, 28, 28);
    ctx.fillStyle = "#7fb069";
    ctx.fillRect(drop.x - 4, drop.y - 11 + bob, 8, 22);
    ctx.fillRect(drop.x - 11, drop.y - 4 + bob, 22, 8);
  }

  function drawTreasure(chest) {
    const bob = Math.sin(chest.life * 5) * 3;
    ctx.fillStyle = "rgba(0, 0, 0, 0.32)";
    ctx.fillRect(chest.x - 24, chest.y + 18, 48, 10);
    ctx.fillStyle = "#5c2f1f";
    ctx.fillRect(chest.x - 24, chest.y - 12 + bob, 48, 32);
    ctx.fillStyle = "#d5a43a";
    ctx.fillRect(chest.x - 24, chest.y - 12 + bob, 48, 9);
    ctx.fillRect(chest.x - 4, chest.y - 12 + bob, 8, 32);
    ctx.fillStyle = "#f1d879";
    ctx.fillRect(chest.x - 2, chest.y + 2 + bob, 4, 6);
    ctx.strokeStyle = "#0a0d0e";
    ctx.lineWidth = 3;
    ctx.strokeRect(chest.x - 24, chest.y - 12 + bob, 48, 32);
  }

  function drawRunChest(chest) {
    const bob = Math.sin(chest.life * 5) * 3;
    const primary = chest.type === "item" ? "#8b5cf6" : "#7fb069";
    const lid = chest.type === "item" ? "#d5a43a" : "#9bd27b";
    ctx.fillStyle = "rgba(0, 0, 0, 0.32)";
    ctx.fillRect(chest.x - 22, chest.y + 18, 44, 9);
    ctx.fillStyle = "#171b1c";
    ctx.fillRect(chest.x - 22, chest.y - 12 + bob, 44, 32);
    ctx.fillStyle = primary;
    ctx.fillRect(chest.x - 22, chest.y - 12 + bob, 44, 9);
    ctx.fillRect(chest.x - 5, chest.y - 12 + bob, 10, 32);
    ctx.fillStyle = lid;
    ctx.fillRect(chest.x - 3, chest.y + 2 + bob, 6, 6);
    ctx.strokeStyle = "#0a0d0e";
    ctx.lineWidth = 3;
    ctx.strokeRect(chest.x - 22, chest.y - 12 + bob, 44, 32);
  }

  function drawAura() {
    if (!player || player.runItems.aura <= 0) return;
    const now = performance.now();
    const pulse = 0.82 + Math.sin(now / 180) * 0.12;
    ctx.save();
    ctx.globalAlpha = 0.22;
    ctx.fillStyle = "#7fb069";
    ctx.beginPath();
    ctx.ellipse(player.x - 4, player.y + 4, 150 * pulse, 108 * pulse, 0, 0, Math.PI * 2);
    ctx.fill();
    ctx.globalAlpha = 0.36;
    for (let i = 0; i < 10; i++) {
      const angle = i * 0.9 + now / 620;
      const radius = 54 + (i % 4) * 24;
      ctx.fillRect(
        Math.round(player.x + Math.cos(angle) * radius),
        Math.round(player.y + Math.sin(angle * 1.2) * radius * 0.65),
        5,
        5,
      );
    }
    ctx.restore();
  }

  function drawFootball() {
    if (!player || player.runItems.football <= 0) return;
    for (let i = 0; i < player.runItems.football; i++) {
      const ball = footballPosition(i);
      ctx.save();
      ctx.translate(ball.x, ball.y);
      ctx.rotate(player.itemTimers.footballAngle * 1.8 + i);
      drawItemImage(footballImage, 44, 44, "#f1ead7");
      ctx.restore();
    }
  }

  function drawItemImage(image, width, height, fallback) {
    if (image.complete && image.naturalWidth) {
      ctx.drawImage(image, -width / 2, -height / 2, width, height);
      return;
    }
    ctx.fillStyle = "#0a0d0e";
    ctx.fillRect(-width / 2, -height / 2, width, height);
    ctx.fillStyle = fallback;
    ctx.fillRect(-width / 2 + 4, -height / 2 + 4, width - 8, height - 8);
  }

  function drawParticle(p) {
    ctx.globalAlpha = Math.max(0, p.life / p.maxLife);
    ctx.fillStyle = p.color;
    ctx.fillRect(Math.round(p.x), Math.round(p.y), p.size, p.size);
    ctx.globalAlpha = 1;
  }

  function drawNumbers() {
    if (!numbers.length) return;
    ctx.font = "900 18px 'Courier New', monospace";
    for (const n of numbers) {
      if (!isCircleVisible(n.x, n.y, 80, 24)) continue;
      const alpha = Math.max(0, n.life);
      ctx.globalAlpha = alpha;
      ctx.fillStyle = "#0a0d0e";
      ctx.fillText(n.text, Math.round(n.x + 2), Math.round(n.y + 2));
      ctx.fillStyle = n.color;
      ctx.fillText(n.text, Math.round(n.x), Math.round(n.y));
    }
    ctx.globalAlpha = 1;
  }

  function updateHud() {
    setCachedStyle(ui.hpFill, "hpWidth", `${clamp(player.hp / player.maxHp, 0, 1) * 100}%`, "width");
    setCachedText(ui.hpText, "hpText", `${Math.max(0, Math.ceil(player.hp))} / ${player.maxHp}`);
    setCachedText(ui.runGold, "gold", player.runGold);
    setCachedText(ui.runKills, "kills", player.kills);
    setCachedText(ui.runLevel, "level", player.level);
    setCachedStyle(ui.xpFill, "xpWidth", `${clamp(player.xp / player.nextLevelXP, 0, 1) * 100}%`, "width");
    setCachedText(ui.xpText, "xpText", `${player.xp} / ${player.nextLevelXP} XP`);
    setCachedText(ui.runTime, "time", formatTime(player.survivalSeconds));

    if (!ui.bossPanel) return;
    const bossVisible = Boolean(boss);
    if (hudCache.bossVisible !== bossVisible) {
      hudCache.bossVisible = bossVisible;
      ui.bossPanel.hidden = !bossVisible;
    }
    if (boss) {
      setCachedStyle(ui.bossFill, "bossWidth", `${clamp(boss.hp / boss.maxHp, 0, 1) * 100}%`, "width");
      setCachedText(ui.bossText, "bossText", `${boss.name} ${Math.max(0, Math.ceil(boss.hp))} / ${boss.maxHp} - ${bossPressureText()}`);
    }
  }

  function bossPressureText() {
    if (!boss) return "";
    const remaining = Math.ceil(bossSpawnGraceSeconds - boss.elapsed);
    if (remaining <= 0) return "Enemies spawning";
    return `Enemies in ${formatTime(remaining)}`;
  }

  function setCachedText(element, key, value) {
    if (!element) return;
    const next = String(value);
    if (hudCache[key] === next) return;
    hudCache[key] = next;
    element.textContent = next;
  }

  function setCachedStyle(element, key, value, property) {
    if (!element || hudCache[key] === value) return;
    hudCache[key] = value;
    element.style[property] = value;
  }

  function burst(x, y, color, count) {
    count = Math.min(count, perf.maxParticles);
    const overflow = particles.length + count - perf.maxParticles;
    if (overflow > 0) particles.splice(0, overflow);
    for (let i = 0; i < count; i++) {
      const angle = Math.random() * Math.PI * 2;
      const speed = 40 + Math.random() * 150;
      const life = 0.25 + Math.random() * 0.45;
      particles.push({
        x,
        y,
        vx: Math.cos(angle) * speed,
        vy: Math.sin(angle) * speed,
        color,
        size: 3 + Math.floor(Math.random() * 4),
        life,
        maxLife: life,
      });
    }
  }

  function makeEnvironment() {
    const details = [];
    const colors = ["#484533", "#3f4132", "#4e463b", "#252a27", "#5c5240"];
    for (let i = 0; i < 180; i++) {
      details.push({
        x: Math.floor(Math.random() * world.width / 16) * 16,
        y: Math.floor(Math.random() * world.height / 16) * 16,
        w: 8 + Math.floor(Math.random() * 4) * 8,
        h: 8 + Math.floor(Math.random() * 3) * 8,
        color: colors[Math.floor(Math.random() * colors.length)],
        mark: Math.random() > 0.62,
      });
    }
    return details;
  }

  function makeWorldCanvas() {
    const buffer = document.createElement("canvas");
    buffer.width = world.width;
    buffer.height = world.height;
    const bufferCtx = buffer.getContext("2d");
    bufferCtx.imageSmoothingEnabled = false;
    bufferCtx.fillStyle = "#303028";
    bufferCtx.fillRect(0, 0, world.width, world.height);

    const tile = 64;
    for (let y = 0; y < world.height; y += tile) {
      for (let x = 0; x < world.width; x += tile) {
        bufferCtx.fillStyle = ((x / tile + y / tile) % 2 === 0) ? "#34332a" : "#2c302b";
        bufferCtx.fillRect(x, y, tile, tile);
      }
    }

    bufferCtx.strokeStyle = "#4b4538";
    bufferCtx.lineWidth = 6;
    bufferCtx.strokeRect(12, 12, world.width - 24, world.height - 24);

    for (const d of rngDetails) {
      bufferCtx.fillStyle = d.color;
      bufferCtx.fillRect(d.x, d.y, d.w, d.h);
      if (d.mark) {
        bufferCtx.fillStyle = "rgba(10, 13, 14, 0.28)";
        bufferCtx.fillRect(d.x + 4, d.y + 4, Math.max(4, d.w - 8), 4);
      }
    }
    return buffer;
  }

  function rebuildEnemyGrid() {
    for (const bucket of enemyGrid.values()) {
      bucket.length = 0;
    }
    for (const enemy of enemies) {
      if (enemy.dead) continue;
      const key = gridKey(enemy.x, enemy.y);
      let bucket = enemyGrid.get(key);
      if (!bucket) {
        bucket = [];
        enemyGrid.set(key, bucket);
      }
      bucket.push(enemy);
    }
  }

  function forEachNearbyEnemy(x, y, radius, callback) {
    const size = perf.enemyCellSize;
    const minX = Math.floor((x - radius) / size);
    const maxX = Math.floor((x + radius) / size);
    const minY = Math.floor((y - radius) / size);
    const maxY = Math.floor((y + radius) / size);
    for (let gy = minY; gy <= maxY; gy++) {
      for (let gx = minX; gx <= maxX; gx++) {
        const bucket = enemyGrid.get(`${gx}:${gy}`);
        if (!bucket) continue;
        for (const enemy of bucket) {
          if (!enemy.dead) callback(enemy);
        }
      }
    }
  }

  function gridKey(x, y) {
    return `${Math.floor(x / perf.enemyCellSize)}:${Math.floor(y / perf.enemyCellSize)}`;
  }

  function pushNumber(number) {
    if (numbers.length >= perf.maxNumbers) numbers.shift();
    numbers.push(number);
  }

  function circleHit(a, b) {
    return circlesOverlap(a.x, a.y, a.r, b.x, b.y, b.r);
  }

  function circlesOverlap(ax, ay, ar, bx, by, br) {
    const dx = ax - bx;
    const dy = ay - by;
    const r = ar + br;
    return distanceSq(dx, dy) < r * r;
  }

  function distanceSq(dx, dy) {
    return dx * dx + dy * dy;
  }

  function isCircleVisible(x, y, radius, extraPadding = perf.drawPadding) {
    return (
      x + radius >= camera.x - extraPadding &&
      y + radius >= camera.y - extraPadding &&
      x - radius <= camera.x + viewWidth() + extraPadding &&
      y - radius <= camera.y + viewHeight() + extraPadding
    );
  }

  function clamp(value, min, max) {
    return Math.max(min, Math.min(max, value));
  }

  function wrapAngle(angle) {
    while (angle < -Math.PI) angle += Math.PI * 2;
    while (angle > Math.PI) angle -= Math.PI * 2;
    return angle;
  }

  function formatTime(seconds) {
    const mins = Math.floor(seconds / 60);
    const secs = Math.floor(seconds % 60);
    return `${mins}:${String(secs).padStart(2, "0")}`;
  }

  async function toggleFullscreen() {
    if (!document.fullscreenElement) {
      await stage.requestFullscreen();
      return;
    }
    await document.exitFullscreen();
  }

  function updateFullscreenButton() {
    const label = document.fullscreenElement === stage ? "Exit Fullscreen" : "Fullscreen";
    fullscreenButtons.forEach((button) => {
      button.textContent = label;
    });
  }

  function wantsGameLoop() {
    return state.running && document.visibilityState !== "hidden";
  }

  function startGameLoop() {
    if (loopActive || !wantsGameLoop()) return;
    loopActive = true;
    state.lastTime = performance.now();
    loopHandle = requestAnimationFrame(loop);
  }

  function stopGameLoop() {
    loopActive = false;
    if (loopHandle) {
      cancelAnimationFrame(loopHandle);
      loopHandle = 0;
    }
  }

  function loop(now) {
    if (!loopActive || !wantsGameLoop()) {
      stopGameLoop();
      return;
    }

    const dt = Math.min(0.033, (now - state.lastTime) / 1000);
    state.lastTime = now;
    update(dt);
    draw();
    loopHandle = requestAnimationFrame(loop);
  }

  document.getElementById("playButton").addEventListener("click", startRun);
  document.getElementById("playAgainButton").addEventListener("click", startRun);
  document.getElementById("mainMenuButton").addEventListener("click", () => showScreen("menu"));
  document.getElementById("continueButton").addEventListener("click", resumeRun);
  document.getElementById("quitButton").addEventListener("click", quitRun);
  fullscreenButtons.forEach((button) => {
    button.addEventListener("click", () => {
      toggleFullscreen().catch(() => {});
    });
  });
  document.getElementById("shopButton").addEventListener("click", () => {
    renderShop();
    showScreen("shop");
  });
  shopCategoryButtons.forEach((button) => {
    button.addEventListener("click", () => {
      state.shopCategory = button.dataset.shopCategory || "stats";
      renderShop();
    });
  });
  document.getElementById("leaderboardButton").addEventListener("click", () => {
    renderLeaderboard();
    showScreen("leaderboard");
    loadLeaderboard();
  });
  leaderboardSortButtons.forEach((button) => {
    button.addEventListener("click", () => loadLeaderboard(button.dataset.leaderboardSort));
  });
  menuBackButtons.forEach((button) => {
    button.addEventListener("click", () => showScreen("menu"));
  });

  window.addEventListener("keydown", (event) => {
    if (["KeyW", "KeyA", "KeyS", "KeyD", "Escape"].includes(event.code)) {
      event.preventDefault();
    }
    if (event.code === "Escape") {
      if (state.paused) resumeRun();
      else pauseRun();
      return;
    }
    if (state.paused) return;
    keys.add(event.code);
  });
  window.addEventListener("keyup", (event) => keys.delete(event.code));
  window.addEventListener("mousemove", (event) => {
    mouse.x = event.clientX;
    mouse.y = event.clientY;
  });
  window.addEventListener("scroll", updateCanvasBounds, { passive: true });
  window.addEventListener("resize", resizeCanvas);
  document.addEventListener("fullscreenchange", () => {
    updateFullscreenButton();
    resizeCanvas();
  });
  document.addEventListener("visibilitychange", () => {
    if (wantsGameLoop()) startGameLoop();
    else stopGameLoop();
  });

  resizeCanvas();
  updateFullscreenButton();
  showScreen("menu");
  loadState().catch(() => {});
})();
