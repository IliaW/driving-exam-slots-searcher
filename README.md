# driving-exam-slots-searcher

A simple application that automatically checks the availability of slots in the electronic order https://eqn.hsc.gov.ua/
for driving exams in Ukraine

<img src="resourses/mvs-login-page.png" alt="mvs-login-page" width="600"/>

## Description

Exam slots can appear randomly at any time on any date. The Chrome browser and the go-rod library are used to automate
this process. Notifications are sent to the phone in the ntfy application. After receiving a notification with
information about the date and location of the exam, you need to manually book the slot.

## How to use

1. Compile the code for your operating system.
2. Set up the search configuration using the `config.yaml` file:
    - Specify the desired dates to search for slots in the `exam_dates` field. Since slots are only available 21 days
      ahead, it is sufficient to use only a number. Separate the selected dates with `;` (“13;14;15”). Note that some
      dates do not have slots as they are weekends in the service.
    - Specify the desired slots in the `addresses` field. You can enter both just the city ("м. Київ") and more precise
      addresses ("м. Київ, вул. Павла Усенка 8"). Separate the selected dates with `;`.

   **IMPORTANT:** the _contains()_ operation is used to search the text on the page, so the string should be exactly
   as on the exam location selection page ("м. Київ, вул. Павла Усенка 8; м. Чернігів").
3. Specify a topic for the Ntfy application in the `ntfy_topic` field. The title should be unique and complex.
4. Download the Ntfy app on your smartphone and add the specified topic. Check the link for more
   information: https://docs.ntfy.sh/.
5. Run the program. The program will open a browser window where you will need to authenticate. After authorization the
   window will be closed and the search will be performed in headless browsers.

**IMPORTANT:** to be able to take the practical exam, you must have successfully completed the theoretical exam
beforehand. Check manually if you have access to the practical exam by simply clicking on the `Практичний іспит` button
on the site https://eqn.hsc.gov.ua/cabinet/queue.

## Limitations

Unfortunately, the service center's website does not have a refresh token. The existing access token has a TTL of about
2 hours. After this time you will need to be re-authorized.
