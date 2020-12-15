package sqldb
import (
	"container/list"
	"gopkg.in/gomail.v2"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const sendmail = "/usr/sbin/sendmail"

// Use sendmail to send emails.
func submitMail(m *gomail.Message) (err error) {
	cmd := exec.Command(sendmail, "-t")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	pw, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	err = cmd.Start()
	if err != nil {
		return err
	}

	_, err = m.WriteTo(pw)
	if err != nil {
		return err
	}

	err = pw.Close()
	if err != nil {
		return err
	}

	err = cmd.Wait()
	if err != nil {
		return err
	}

	return err
}



// Send out the certificate informations to the email monitoring them.
func SendEmail(email string, certList *list.List) {
	if email == "" {
		return
	}
	
	t := time.Now().Add(-24 * time.Hour)
	date := strings.Join([]string{strconv.Itoa(t.Day()), strconv.Itoa(int(t.Month())), strconv.Itoa(t.Year())}, ".")

	m := gomail.NewMessage()
	m.SetHeader("From", "no-reply@cesnet.cz")
	m.SetHeader("To", email)
	m.SetHeader("Subject", "[CTLog] Nové certifikáty " + date)

	var sb strings.Builder

	sb.WriteString(emailConst)

	for cert := certList.Front(); cert != nil; cert = cert.Next() {
		sb.WriteString("<ul>")
		cur := cert.Value.(CertInfo)
		sb.WriteString(cur.CN)
		sb.WriteString("<li>Subject DN: " + cur.DN + "</li>" +
			"<li>Serial: " + cur.SerialNumber + "</li>" +
			"<li>Names: " + cur.SAN + "</li>")
		sb.WriteString("</ul>")
	}

	sb.WriteString("<a href=\"pki.cesnet.cz\">O službě</a>")
	sb.WriteString("<img src=\"https://www.cesnet.cz/wp-content/uploads/2018/01/cesnet-malelogo.jpg\"><br></body>")
	m.SetBody("text/html", sb.String())

	if err := submitMail(m); err != nil {
		log.Printf("[-] Failed sending email to %s -> %s", email, err)
	}
}