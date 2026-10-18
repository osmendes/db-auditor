# Fonte embutida nos PDFs

`LiberationSans-Regular.ttf` é o arquivo original, sem modificação, da Liberation Sans 2.1.5 (pacote Debian `fonts-liberation` 1:2.1.5-3). O sha256 é `bade59d822652f76e6941aa87b40a87c13d1cc70db98ededb5011127efafd1d3`. A licença é a SIL Open Font License 1.1, em [LICENSE-LiberationSans.txt](LICENSE-LiberationSans.txt).

A fonte tem as mesmas métricas da Helvetica/Arial, por isso a quebra de linhas e a paginação do relatório não mudaram ao trocar a Helvetica base-14 pela fonte embutida (#107).

O gerador (`internal/report/font.go`) embute no PDF um subconjunto com os glifos da WinAnsiEncoding. O `fsType` da fonte é 0, ou seja, a fonte permite embutir. A OFL também permite embutir subconjuntos em documentos, e o documento gerado não fica sujeito à OFL.
