import { Component, OnInit } from '@angular/core';
import { NgbActiveModal } from '@ng-bootstrap/ng-bootstrap';


@Component({
  selector: 'app-alert',
  templateUrl: './alert.component.html',
  styleUrls: ['./alert.component.scss'],
})

export class AlertComponent implements OnInit {
  title = '';
  body = '';

  constructor(public activeModal: NgbActiveModal) {
  }

  ngOnInit() {
  }
}
