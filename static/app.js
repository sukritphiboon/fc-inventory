let pollTimer = null;

// ── LocalStorage: remember last used credentials ────────
function saveCredentials() {
    const host = document.getElementById("host").value.trim();
    const port = document.getElementById("port").value;
    const username = document.getElementById("username").value.trim();
    if (host) localStorage.setItem("fc_host", host);
    if (port) localStorage.setItem("fc_port", port);
    if (username) localStorage.setItem("fc_username", username);
}

function loadCredentials() {
    const host = localStorage.getItem("fc_host");
    const port = localStorage.getItem("fc_port");
    const username = localStorage.getItem("fc_username");
    if (host) document.getElementById("host").value = host;
    if (port) document.getElementById("port").value = port;
    if (username) document.getElementById("username").value = username;
}

// ── Enter key to submit ─────────────────────────────────
function handleKeyDown(e) {
    if (e.key === "Enter") {
        e.preventDefault();
        startCollection();
    }
}

// ── Init on page load ───────────────────────────────────
window.addEventListener("DOMContentLoaded", function () {
    loadCredentials();

    // Attach Enter key listener to all form inputs
    document.querySelectorAll("#form-section input").forEach(function (input) {
        input.addEventListener("keydown", handleKeyDown);
    });

    // Focus password if host/username already filled, otherwise focus host
    if (document.getElementById("host").value && document.getElementById("username").value) {
        document.getElementById("password").focus();
    } else {
        document.getElementById("host").focus();
    }
});

// ── Demo Mode ───────────────────────────────────────────
function startDemo() {
    hideFormError();

    fetch("/api/collect", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ host: "mock", username: "mock", password: "mock", mock: true }),
    })
        .then((resp) => {
            if (!resp.ok) return resp.json().then((d) => Promise.reject(d));
            return resp.json();
        })
        .then(() => {
            document.getElementById("form-section").style.display = "none";
            document.getElementById("progress-section").style.display = "block";
            document.getElementById("result-section").style.display = "none";
            pollTimer = setInterval(pollProgress, 1000);
        })
        .catch((err) => {
            showFormError(err.error || "Failed to start demo.");
        });
}

// ── Real Collection ─────────────────────────────────────
function startCollection() {
    const host = document.getElementById("host").value.trim();
    const port = parseInt(document.getElementById("port").value) || 7443;
    const username = document.getElementById("username").value.trim();
    const password = document.getElementById("password").value;

    if (!host || !username || !password) {
        showFormError("Please fill in all fields.");
        return;
    }

    hideFormError();
    saveCredentials();
    document.getElementById("btn-collect").disabled = true;

    fetch("/api/collect", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ host, port, username, password }),
    })
        .then((resp) => {
            if (!resp.ok) return resp.json().then((d) => Promise.reject(d));
            return resp.json();
        })
        .then(() => {
            document.getElementById("form-section").style.display = "none";
            document.getElementById("progress-section").style.display = "block";
            document.getElementById("result-section").style.display = "none";
            pollTimer = setInterval(pollProgress, 2000);
        })
        .catch((err) => {
            document.getElementById("btn-collect").disabled = false;
            showFormError(err.error || "Failed to start collection.");
        });
}

// ── Progress Polling ────────────────────────────────────
function pollProgress() {
    fetch("/api/progress")
        .then((resp) => resp.json())
        .then((data) => {
            const pct = data.percent || 0;
            document.getElementById("progress-bar").style.width = pct + "%";
            document.getElementById("percent-text").textContent = pct + "%";
            document.getElementById("step-text").textContent =
                data.current_step || "Working...";

            if (data.status === "done") {
                clearInterval(pollTimer);
                showSuccess();
            } else if (data.status === "cancelled") {
                clearInterval(pollTimer);
                resetForm();
            } else if (data.status === "error") {
                clearInterval(pollTimer);
                showError(data.error || "An unknown error occurred.");
            }
        })
        .catch(() => {
            document.getElementById("step-text").textContent =
                "Connection lost, retrying...";
        });
}

// ── UI State Changes ────────────────────────────────────
function showSuccess() {
    document.getElementById("progress-section").style.display = "none";
    document.getElementById("result-section").style.display = "block";
    document.getElementById("success-box").style.display = "block";
    document.getElementById("error-box").style.display = "none";
}

function showError(message) {
    document.getElementById("progress-section").style.display = "none";
    document.getElementById("result-section").style.display = "block";
    document.getElementById("success-box").style.display = "none";
    document.getElementById("error-box").style.display = "block";
    document.getElementById("error-detail").textContent = message;
}

function cancelCollection() {
    fetch("/api/cancel", { method: "POST" });
    document.getElementById("step-text").textContent = "Cancelling...";
}

function resetForm() {
    document.getElementById("form-section").style.display = "block";
    document.getElementById("progress-section").style.display = "none";
    document.getElementById("result-section").style.display = "none";

    // Keep host/port/username (don't clear), only clear password
    document.getElementById("password").value = "";
    document.getElementById("btn-collect").disabled = false;

    document.getElementById("progress-bar").style.width = "0%";
    document.getElementById("percent-text").textContent = "0%";

    hideFormError();
    document.getElementById("password").focus();
}

function showFormError(msg) {
    const el = document.getElementById("form-error");
    el.textContent = msg;
    el.style.display = "block";
}

function hideFormError() {
    document.getElementById("form-error").style.display = "none";
}
