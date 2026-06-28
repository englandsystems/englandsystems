const transitionLinks = 'a[data-transition-link]';
const sectionIds = ["home", "about", "contact"];

function getMain(documentRoot = document) {
  return documentRoot.querySelector("main");
}

function shouldHandleNavigation(event, link) {
  if (
    event.defaultPrevented ||
    event.button !== 0 ||
    event.metaKey ||
    event.ctrlKey ||
    event.shiftKey ||
    event.altKey ||
    link.target
  ) {
    return false;
  }

  return link.origin === window.location.origin && link.pathname !== window.location.pathname;
}

async function fetchPage(url) {
  const response = await fetch(url, {
    headers: {
      Accept: "text/html",
    },
  });

  if (!response.ok) {
    throw new Error(`Unable to load ${url}`);
  }

  const html = await response.text();
  return new DOMParser().parseFromString(html, "text/html");
}

function swapPage(nextDocument, url, shouldPushState) {
  const currentMain = getMain();
  const nextMain = getMain(nextDocument);

  if (!currentMain || !nextMain) {
    window.location.href = url;
    return;
  }

  document.title = nextDocument.title;
  currentMain.className = nextMain.className;
  currentMain.innerHTML = nextMain.innerHTML;

  if (shouldPushState) {
    window.history.pushState({}, "", url);
  }

  initContactEnhancements();
}

async function navigate(url, shouldPushState = true) {
  const nextDocument = await fetchPage(url);

  if (!document.startViewTransition) {
    swapPage(nextDocument, url, shouldPushState);
    return;
  }

  document.startViewTransition(() => {
    swapPage(nextDocument, url, shouldPushState);
  });
}

document.addEventListener("click", (event) => {
  const link = event.target.closest(transitionLinks);

  if (!link || !shouldHandleNavigation(event, link)) {
    return;
  }

  event.preventDefault();
  navigate(link.href).catch(() => {
    window.location.href = link.href;
  });
});

window.addEventListener("popstate", () => {
  if (document.querySelector("[data-section-panel]")) {
    setActiveSection(window.location.hash || "home", false);
    return;
  }

  navigate(window.location.href, false).catch(() => {
    window.location.reload();
  });
});

function updateCharacterCounter(textarea) {
  const counter = textarea.parentElement?.querySelector("[data-character-count]");
  const maxLength = Number(textarea.getAttribute("maxlength"));

  if (!counter || !maxLength) {
    return;
  }

  const remaining = Math.max(0, maxLength - textarea.value.length);
  counter.textContent = `${remaining} character${remaining === 1 ? "" : "s"} left`;
}

function initContactEnhancements() {
  document.querySelectorAll("[data-character-counter]").forEach(updateCharacterCounter);

  document.querySelectorAll("[data-contact-form]").forEach((form) => {
    if (form.dataset.validationReady === "true") {
      return;
    }

    form.dataset.validationReady = "true";
    form.addEventListener("input", (event) => validateContactField(event.target));
    form.addEventListener("submit", (event) => {
      form.querySelectorAll("input, textarea").forEach(validateContactField);
      if (!form.checkValidity()) {
        event.preventDefault();
        form.reportValidity();
      }
    });
  });

  const success = document.querySelector("[data-contact-success]");
  if (success) {
    success.hidden = new URLSearchParams(window.location.search).get("sent") !== "1";
  }
}

function validateContactField(field) {
  if (!(field instanceof HTMLInputElement || field instanceof HTMLTextAreaElement)) {
    return;
  }

  field.setCustomValidity("");
  const value = field.value.trim();

  if (field.required && value === "") {
    field.setCustomValidity("This field is required.");
    return;
  }

  if (field.name === "name" && value !== "" && !/\p{L}/u.test(value)) {
    field.setCustomValidity("Enter a name containing at least one letter.");
  }

  if (field.name === "message" && value !== "" && !/[\p{L}\p{N}]/u.test(value)) {
    field.setCustomValidity("Enter a message containing text.");
  }

  if (field.name === "email" && value !== "") {
    const emailPattern = /^[^\s@]+@[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?)+$/;
    if (!emailPattern.test(value)) {
      field.setCustomValidity("Enter a valid email address.");
    }
  }

  if (field.name === "phone" && value !== "") {
    const digits = value.replace(/\D/g, "");
    const hasValidCharacters = /^\+?[0-9 ().-]+$/.test(value);
    const hasBalancedParentheses = !/[()]/.test(value) || /^\+?[^()]*(\([^()]+\)[^()]*)+$/.test(value);
    const allDigitsMatch = /^(\d)\1+$/.test(digits);
    if (!hasValidCharacters || !hasBalancedParentheses || digits.length < 7 || digits.length > 15 || allDigitsMatch) {
      field.setCustomValidity("Enter a valid phone number with 7 to 15 digits.");
    }
  }
}

function normalizeSectionId(hash) {
  const sectionId = hash.replace(/^#/, "");
  return sectionIds.includes(sectionId) ? sectionId : "home";
}

function setActiveSection(sectionId, shouldPushState = true) {
  const activeSection = normalizeSectionId(sectionId);
  const panels = document.querySelectorAll("[data-section-panel]");

  if (!panels.length) {
    return;
  }

  document.body.dataset.activeSection = activeSection;

  panels.forEach((panel) => {
    const isActive = panel.id === activeSection;
    panel.hidden = false;
    panel.setAttribute("aria-hidden", String(!isActive));

    if (isActive) {
      window.requestAnimationFrame(() => {
        panel.classList.add("is-active");
      });
    } else {
      panel.classList.remove("is-active");
      window.setTimeout(() => {
        if (!panel.classList.contains("is-active")) {
          panel.hidden = true;
        }
      }, 380);
    }
  });

  document.querySelectorAll("[data-section-link]").forEach((link) => {
    const linkSection = normalizeSectionId(link.hash);
    const isCurrent = linkSection === activeSection;

    if (isCurrent) {
      link.setAttribute("aria-current", "page");
    } else {
      link.removeAttribute("aria-current");
    }
  });

  if (shouldPushState) {
    const nextUrl = activeSection === "home" ? window.location.pathname : `#${activeSection}`;
    window.history.pushState({ sectionId: activeSection }, "", nextUrl);
  }

  window.dispatchEvent(new CustomEvent("sectionchange", { detail: { sectionId: activeSection } }));
}

function initSectionNavigation() {
  const panels = document.querySelectorAll("[data-section-panel]");

  if (!panels.length) {
    return;
  }

  document.addEventListener("click", (event) => {
    const link = event.target.closest("[data-section-link]");

    if (!link || link.pathname !== window.location.pathname || link.origin !== window.location.origin) {
      return;
    }

    event.preventDefault();
    setActiveSection(link.hash);
  });

  setActiveSection(window.location.hash || "home", false);
}

document.addEventListener("input", (event) => {
  if (event.target.matches("[data-character-counter]")) {
    updateCharacterCounter(event.target);
  }
});

window.addEventListener("hashchange", () => {
  setActiveSection(window.location.hash || "home", false);
});

initContactEnhancements();
initSectionNavigation();
