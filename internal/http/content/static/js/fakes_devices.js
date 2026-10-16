class FakeDevices {
    constructor(doc) {
        this.doc = doc;
    }

    init = () => {
        const root = this.doc.querySelector("div.entity-fakes-devices");

        if (!root) {
            return false;
        }

        root.querySelectorAll("button.fake-button").forEach((button) => {
            button.addEventListener("click", () => this.act(button));
        });

        return true;
    }

    act = async (button) => {
        const card = button.closest(".fake-device");
        const errorElement = card.querySelector(".action-error");
        const action = button.dataset.action;
        errorElement.textContent = "";
        button.disabled = true;

        const body = {Name: card.dataset.name, Action: action};

        if (action === "fail-next") {
            body.Count = parseInt(card.querySelector("input.fail-count").value, 10) || 0;
        } else if (action === "set-temperature") {
            body.Value = parseFloat(card.querySelector("input.temperature").value);
        } else if (action === "adjust-temperature") {
            body.Value = parseFloat(button.dataset.delta);
        }

        try {
            const response = await fetch("/plugins/fakes/control", {
                method: "POST",
                headers: {"Content-Type": "application/json"},
                body: JSON.stringify(body),
            });

            if (!response.ok) {
                // Say so on the page: a click that silently does nothing is worse than an error.
                console.error("Failed to change the fake:", response.status, await response.text());
                errorElement.textContent = "Could not do that (" + response.status + ")";
                button.disabled = false;
                return;
            }

            location.reload();
        } catch (error) {
            console.error("Failed to change the fake:", error);
            errorElement.textContent = "Could not do that: " + error;
            button.disabled = false;
        }
    }
}

new FakeDevices(document).init();
