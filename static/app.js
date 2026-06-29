function updateCharacterCounter(textarea) {
  const counter = textarea.parentElement?.querySelector("[data-character-count]");
  const maxLength = Number(textarea.getAttribute("maxlength"));

  if (!counter || !maxLength) return;

  const remaining = Math.max(0, maxLength - textarea.value.length);
  counter.textContent = `${remaining} character${remaining === 1 ? "" : "s"} left`;
}

function validateContactField(field) {
  if (!(field instanceof HTMLInputElement || field instanceof HTMLTextAreaElement)) return;

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
    if (!emailPattern.test(value)) field.setCustomValidity("Enter a valid email address.");
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

function initContactEnhancements() {
  document.querySelectorAll("[data-character-counter]").forEach(updateCharacterCounter);

  document.querySelectorAll("[data-contact-form]").forEach((form) => {
    form.addEventListener("input", (event) => {
      validateContactField(event.target);
      if (event.target.matches("[data-character-counter]")) updateCharacterCounter(event.target);
    });

    form.addEventListener("submit", (event) => {
      form.querySelectorAll("input, textarea").forEach(validateContactField);
      if (!form.checkValidity()) {
        event.preventDefault();
        form.reportValidity();
      }
    });
  });

  const success = document.querySelector("[data-contact-success]");
  if (success) success.hidden = new URLSearchParams(window.location.search).get("sent") !== "1";
}

function initSectionTracking() {
  const links = [...document.querySelectorAll('.site-nav a[href^="#"]')];
  if (!links.length || !("IntersectionObserver" in window)) return;

  const sections = links
    .map((link) => document.querySelector(link.getAttribute("href")))
    .filter(Boolean);

  const observer = new IntersectionObserver((entries) => {
    const visible = entries
      .filter((entry) => entry.isIntersecting)
      .sort((a, b) => b.intersectionRatio - a.intersectionRatio)[0];

    if (!visible) return;
    document.body.dataset.activeSection = visible.target.id;
    links.forEach((link) => {
      if (link.getAttribute("href") === `#${visible.target.id}`) link.setAttribute("aria-current", "page");
      else link.removeAttribute("aria-current");
    });
  }, { rootMargin: "-25% 0px -55%", threshold: [0, 0.1, 0.3] });

  sections.forEach((section) => observer.observe(section));
}

initContactEnhancements();
initSectionTracking();
