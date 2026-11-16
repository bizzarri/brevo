Brevo Club Mail 0.5.0 for Windows x64
===================================

Extract the ZIP and double-click Brevo-Club-Mail.exe. Reuse your
existing profile JSON by keeping it next to the executable. For a new
setup, copy club-mail.example.json to club-mail.json and enter the
Brevo API key, verified sender, reply-to address, test address, and
mailing list IDs. Use one JSON file per account, for example amador.json
and caldc.json. Click Reload profiles after making changes.

Using your Brevo templates
--------------------------

In Brevo, put [Insert body here] by itself in a text block, exactly
once. Paste or type the entire marker in the same style so the
editor does not split it into multiple HTML elements. Leave the
existing address and unsubscribe link at the bottom.

Add a templates entry to your local profile, for example:

    "templates": {
      "123": "Amador newsletter",
      "456": "Meeting reminder"
    }

Replace 123 and 456 with your actual Brevo template IDs. The app uses
Brevo's email template API to fetch the HTML when you preview and replace the marker with your message. A Restricted API
key needs read access to Transactional email for this template lookup,
plus the ability to create and send Email campaigns.

For a design unavailable through that API, save its HTML as
newsletter.html next to your profile and use the filename as the key:

    "templates": {"newsletter.html": "Amador newsletter"}

Select your template and list, then click Preview email. Your default
browser opens the assembled HTML, including the design and footer.
View HTML reopens it. The app uses this same HTML for tests and sends
until you return to Edit and preview again. It creates temporary
preview files and removes them when the app closes normally. The
preview shows the HTML before Brevo processes recipient fields and
links. For the final recipient view, open a delivered test email.

The app uses the profile sender and your typed subject. The template
supplies the address and unsubscribe link, so its profile does not
need a footer setting. With a footer, Plain email is also available.

If Brevo reports an error, the status area includes the failed step
and Brevo's message when available. You can select and copy it. If
Brevo did not confirm the send, check for the campaign by its ID
before starting a new attempt. The app blocks repeat attempts for the
same exact draft and action in the current session.

Brevo access
------------

The local profile controls the lists shown in the app. For Brevo to
enforce that separation, give the president a key from an account that
contains only the approved Amador lists. Your CalDC profile and key
can stay on your own PC. The profile's api_key can be replaced by
api_key_env, the name of a Windows environment variable holding the key.

Building from source
--------------------

From the repository root in Command Prompt:

    go build -trimpath -ldflags="-H windowsgui" -o Brevo-Club-Mail.exe .

No Python, Go installation, or relay is needed to run the supplied EXE.
