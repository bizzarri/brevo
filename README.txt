Brevo Club Mail 0.6.0 for Windows x64
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

Sending a meeting packet
------------------------

For member notices such as agendas and minutes, click Attach files
and select all the local files for the packet. Use Ctrl or Shift to
select multiple files in the picker. Files are limited to 12 MB
combined. Click Clear to return to the regular campaign workflow.

Preview the email and click Send test. After Brevo accepts the test,
check that all your attachments arrived, then click Send packet.
The app sends a separate message to each eligible member, skipping
Brevo contacts who are email-blocklisted or have opted out of the
selected list. Brevo also applies its transactional blocklist.
Your profile key needs permission to read contacts and send
transactional email. Check Brevo's transactional logs before retrying
if a packet send could only be partially confirmed.

Managing mailing lists
----------------------

Open Brevo-List-Manager.exe from the same folder as your Club Mail
profile JSON, or click Manage mailing lists in Club Mail. Choose an
account profile and mailing list. Click List members to browse 20 at a
time with Previous and Next. Select a member to see their details.
Enter part of a name and click Search to find a specific member. You can also enter an exact email address and click Look up.
The contact details show list membership and email subscription status.

To add someone, enter their email address and click Look up. Enter
a first or last name if Brevo has none, then click Add to list. If
they already belong to the list and have no name, click Save name. Remove from list removes
only the selected membership. Unsubscribe email keeps the contact on
their lists and blocks email campaigns across that Brevo account.
Adding an existing contact does not clear their email blocklist status.

Use a Brevo API key that can read contacts for lookup and create or
update contacts for list changes.

Building from source
--------------------

From the repository root in Command Prompt:

    go build -trimpath -ldflags="-H windowsgui" -o Brevo-Club-Mail.exe .

No Python, Go installation, or relay is needed to run the supplied EXE.

Build the standalone list manager from the same source:

    go build -trimpath -ldflags="-H windowsgui -X main.listManagerMode=1" -o Brevo-List-Manager.exe .
